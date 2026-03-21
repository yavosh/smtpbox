package dns

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/miekg/dns"
)

var domainsToAddresses = map[string]string{
	"example.com.":      "127.0.0.1",
	"example.org.":      "127.0.0.1",
	"example.net.":      "127.0.0.1",
	"mail.example.net.": "127.0.0.1",
}

type Server struct {
	port   int
	domain string
	server *dns.Server
	log    *slog.Logger
}

type Option func(*Server)

func NewServer(port int, domain string, opts ...Option) *Server {
	s := &Server{
		port:   port,
		domain: domain,
		log:    slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.server = &dns.Server{Addr: fmt.Sprintf(":%d", s.port), Net: "udp"}
	s.server.Handler = s
	return s
}

func (s *Server) Start() error {
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

	domain := msg.Question[0].Name
	s.log.Debug("dns query", "name", domain, "type", r.Question[0].Qtype)

	switch r.Question[0].Qtype {
	case dns.TypeMX:
		msg.Authoritative = true
		if domain == s.domain || domain == s.domain+"." {
			msg.Extra = append(msg.Extra, &dns.MX{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeMX,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				Preference: 10,
				Mx:         "mail." + s.domain + ".",
			})
		}
	case dns.TypeA:
		msg.Authoritative = true
		address, ok := domainsToAddresses[domain]
		if ok {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP(address),
			})
		}
	}

	_ = w.WriteMsg(&msg)
}
