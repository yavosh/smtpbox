// Package dns handles dns aspects of the code
package dns

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/miekg/dns"

	"github.com/yavosh/smtpbox"
)

var domainsToAddresses = map[string]string{
	"example.com.":      "127.0.0.1",
	"example.org.":      "127.0.0.1",
	"example.net.":      "127.0.0.1",
	"mail.example.net.": "127.0.0.1",
}

// Server implements a dns server which you can use to resolve mx records for smtpbox
type Server struct {
	port   int
	domain string
	server *dns.Server
	log    smtpbox.Logger
}

// Option is an option to the server
type Option func(*Server)

func NewServer(port int, domain string, opts ...Option) *Server {
	s := &Server{
		port:   port,
		domain: domain,
		log:    log.New(os.Stdout, "dns ", log.LstdFlags),
	}

	// Loop through each option
	for _, opt := range opts {
		opt(s)
	}

	s.server = &dns.Server{Addr: fmt.Sprintf(":%d", s.port), Net: "udp"}
	s.server.Handler = s
	return s
}

func (s *Server) Start() error {
	s.log.Printf("Starting dns server @ %d ", s.port)
	s.log.Printf("Using dns domain %s", s.domain)

	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.log.Printf("Failed to set udp listener %s\n", err.Error())
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	s.log.Printf("Stopping dns server @ %d ", s.port)
	return s.server.Shutdown()
}

func (s Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)

	s.log.Printf("DNS Query %v", r)
	s.log.Printf("msg.Question[0].Name %s", msg.Question[0].Name)

	switch r.Question[0].Qtype {
	case dns.TypeMX:
		msg.Authoritative = true
		domain := msg.Question[0].Name

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
		domain := msg.Question[0].Name
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
