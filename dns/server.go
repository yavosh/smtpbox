package dns

import (
	"errors"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

type Server struct {
	port       int
	listenHost string
	domain     string
	server     *dns.Server
	log        *slog.Logger
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
		log:        slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.server = &dns.Server{Addr: net.JoinHostPort(s.listenHost, strconv.Itoa(s.port)), Net: "udp"}
	s.server.Handler = s
	return s
}

func (s *Server) Start() error {
	if s.listenHost == "" {
		return errors.New("listen host must not be empty")
	}
	s.log.Info("starting dns server", "port", s.port, "domain", s.domain)

	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.log.Error("failed to start dns listener", "error", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	s.log.Info("stopping dns server", "port", s.port)
	return s.server.Shutdown()
}

func (s Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
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
