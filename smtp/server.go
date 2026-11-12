package smtp

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox"
)

type Server struct {
	mu           sync.Mutex
	listenAddr   string
	domain       string
	instance     *smtp.Server
	listener     net.Listener
	serveDone    chan error
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

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.instance != nil {
		return errors.New("can't start a running server")
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
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("smtp listen: %w", err)
	}

	done := make(chan error, 1)
	s.instance = instance
	s.listener = listener
	s.serveDone = done

	go func() {
		err := instance.Serve(listener)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.log.Error("smtp server error", "error", err)
		}
		done <- err
	}()

	s.log.Info("started smtp server", "addr", s.listenAddr)
	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.instance == nil {
		return errors.New("can't stop, server not running")
	}
	s.log.Info("stopping smtp server")

	err := s.instance.Close()
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	if closeErr := s.listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		err = errors.Join(err, closeErr)
	}
	if serveErr := <-s.serveDone; serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
		err = errors.Join(err, serveErr)
	}
	s.instance = nil
	s.listener = nil
	s.serveDone = nil
	return err
}
