package smtp

import (
	"errors"
	"log/slog"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox"
)

type Server struct {
	listenAddr   string
	domain       string
	instance     *smtp.Server
	emailService smtpbox.EmailStoreBackend
	log          *slog.Logger
}

type Option func(*Server)

func NewServer(listenAddr string, domain string, svc smtpbox.EmailStoreBackend, opts ...Option) *Server {
	s := &Server{
		listenAddr:   listenAddr,
		domain:       domain,
		log:          slog.Default(),
		emailService: svc,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Server) Start() {
	if s.instance != nil {
		s.log.Error("can't start a running server")
		return
	}

	s.log.Info("starting smtp server", "addr", s.listenAddr, "domain", s.domain)

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

	s.log.Info("started smtp server", "addr", s.listenAddr)
}

func (s *Server) Stop() error {
	if s.instance == nil {
		return errors.New("can't stop, server not running")
	}
	s.log.Info("stopping smtp server")

	err := s.instance.Close()
	s.instance = nil
	return err
}
