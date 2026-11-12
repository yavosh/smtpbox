package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

type Server struct {
	mu         sync.Mutex
	port       int
	listenHost string
	domain     string
	server     *dns.Server
	log        *slog.Logger
	done       chan error
}

type Option func(*Server)

func WithListenHost(host string) Option {
	return func(s *Server) { s.listenHost = host }
}

func NewServer(port int, domain string, opts ...Option) *Server {
	s := &Server{
		port:       port,
		listenHost: "127.0.0.1",
		domain:     dns.CanonicalName(domain),
		log:        clogDNS(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.server = &dns.Server{Addr: net.JoinHostPort(s.listenHost, strconv.Itoa(s.port)), Net: "udp"}
	s.server.Handler = s
	return s
}

func (s *Server) Start() error {
	return s.StartContext(context.Background())
}

// StartContext binds the UDP listener and waits until the server is ready.
// The context limits startup; Stop controls the running server.
func (s *Server) StartContext(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start dns server: %w", err)
	}
	if s.done != nil {
		return errors.New("dns server already started")
	}
	if s.listenHost == "" {
		return errors.New("listen host must not be empty")
	}
	address := net.JoinHostPort(s.listenHost, strconv.Itoa(s.port))
	packet, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", address)
	if err != nil {
		return fmt.Errorf("listen for dns: %w", err)
	}
	started := make(chan struct{})
	s.server = &dns.Server{
		Addr:              address,
		Net:               "udp",
		PacketConn:        packet,
		Handler:           s,
		NotifyStartedFunc: func() { close(started) },
	}
	s.done = make(chan error, 1)
	server, done := s.server, s.done
	s.log.Info("starting dns server", "port", s.port, "domain", s.domain)

	go func() { done <- server.ActivateAndServe() }()

	select {
	case <-started:
		if err := ctx.Err(); err != nil {
			_ = server.Shutdown()
			serveErr := <-done
			s.done = nil
			return fmt.Errorf("start dns server: %w", errors.Join(err, serveErr))
		}
		return nil
	case err := <-done:
		_ = packet.Close()
		s.done = nil
		return fmt.Errorf("start dns server: %w", err)
	case <-ctx.Done():
		_ = packet.Close()
		serveErr := <-done
		_ = server.Shutdown()
		s.done = nil
		return fmt.Errorf("start dns server: %w", errors.Join(ctx.Err(), serveErr))
	}
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done == nil {
		return s.server.Shutdown()
	}
	s.log.Info("stopping dns server", "port", s.port)
	err := s.server.Shutdown()
	serveErr := <-s.done
	s.done = nil
	if err != nil {
		return fmt.Errorf("stop dns server: %w", err)
	}
	if serveErr != nil {
		return fmt.Errorf("serve dns: %w", serveErr)
	}
	return nil
}

// Addr returns the bound UDP address, or nil when the server is stopped.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done == nil {
		return nil
	}
	return s.server.PacketConn.LocalAddr()
}

func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)

	question := r.Question[0]
	domain := dns.CanonicalName(question.Name)
	mailHost := dns.Fqdn("mail." + strings.TrimSuffix(s.domain, "."))
	s.log.Debug("dns query", "name", question.Name, "type", question.Qtype)

	if question.Qclass != dns.ClassINET || !dns.IsSubDomain(s.domain, domain) {
		msg.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(&msg)
		return
	}
	msg.Authoritative = true
	if domain != s.domain && domain != mailHost {
		msg.Rcode = dns.RcodeNameError
		_ = w.WriteMsg(&msg)
		return
	}

	header := dns.RR_Header{
		Name:   question.Name,
		Rrtype: question.Qtype,
		Class:  dns.ClassINET,
		Ttl:    60,
	}
	switch question.Qtype {
	case dns.TypeMX:
		if domain == s.domain {
			msg.Answer = append(msg.Answer, &dns.MX{
				Hdr:        header,
				Preference: 10,
				Mx:         mailHost,
			})
		}
	case dns.TypeA:
		msg.Answer = append(msg.Answer, &dns.A{Hdr: header, A: net.IPv4(127, 0, 0, 1)})
	}

	_ = w.WriteMsg(&msg)
}
