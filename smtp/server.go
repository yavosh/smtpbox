package smtp

import (
	"log"
	"os"
	"time"

	"errors"
	"github.com/emersion/go-smtp"
	"github.com/yavosh/smtpbox"
	"github.com/yavosh/smtpbox/domain/email"
)

type Server struct {
	listenAddr        string
	domain            string
	additionalDomains []string
	instance          *smtp.Server
	emailService      email.Backend
	log               smtpbox.Logger
}

// Option is an option to the server
type Option func(*Server)

// NewServer is the constructor for the smtp agent
func NewServer(listenAddr string, domain string, svc email.Backend, opts ...Option) *Server {
	s := &Server{
		listenAddr:        listenAddr,
		domain:            domain,
		additionalDomains: make([]string, 0),
		log:               log.New(os.Stdout, "smtp ", log.LstdFlags),
		emailService:      svc,
	}

	// Loop through each option
	for _, opt := range opts {
		opt(s)
	}

	return s
}

func WithAdditionalDomain(d string) Option {
	return func(h *Server) {
		h.additionalDomains = append(h.additionalDomains, d)
	}
}

func (s *Server) Start() {
	if s.instance != nil {
		s.log.Fatalf("Can't start a running server")
		return
	}

	s.log.Printf("Starting smtp server @ %s ", s.listenAddr)
	s.log.Printf("Using smtp domain %s", s.domain)

	instance := smtp.NewServer(&backend{server: s})
	instance.Addr = s.listenAddr
	instance.Domain = s.domain
	instance.ReadTimeout = 10 * time.Second
	instance.WriteTimeout = 10 * time.Second
	instance.MaxMessageBytes = 1024 * 1024
	instance.MaxRecipients = 50
	instance.AllowInsecureAuth = true
	s.instance = instance

	go func() {
		if err := s.instance.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	s.log.Printf("Started smtp server @ %s ", s.listenAddr)
}

func (s *Server) Stop() error {
	if s.instance == nil {
		return errors.New("can't stop, server not running")
	}
	s.log.Printf("Stopping smtp server")

	err := s.instance.Close()
	s.instance = nil
	return err
}
