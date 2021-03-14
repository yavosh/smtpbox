package smtp

import (
	"errors"
	"github.com/emersion/go-smtp"
	"github.com/yavosh/smtpmocker/agents"
	"io"
	"io/ioutil"
	"log"
	"time"
)

// The Backend implements SMTP server methods.
type backend struct{}

// A Session is returned after successful login.
type session struct {
	username string
	from     string
	rcpt     []string
}

// Login handles a login command with username and password.
func (b *backend) Login(_ *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	if password != "password" {
		return nil, errors.New("invalid username or password")
	}

	return &session{username: username, rcpt: make([]string, 0)}, nil
}

// AnonymousLogin requires clients to authenticate using SMTP AUTH before sending emails
func (b *backend) AnonymousLogin(_ *smtp.ConnectionState) (smtp.Session, error) {
	return nil, smtp.ErrAuthRequired
}

func (s *session) Mail(from string, _ smtp.MailOptions) error {
	log.Println("Mail from:", from)
	s.from = from
	return nil
}

func (s *session) Rcpt(to string) error {
	log.Println("Rcpt to:", to)
	s.rcpt = append(s.rcpt, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	if b, err := ioutil.ReadAll(r); err != nil {
		return err
	} else {
		log.Println("Data:", string(b))
	}
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.rcpt = make([]string, 0)
}

func (s *session) Logout() error {
	log.Println("session ended")
	return nil
}

type server struct {
	listenAddr string
	domain     string
	instance   *smtp.Server
}

// Option is an option to the server
type Option func(*server)

const (
	defaultListenAddr = ":1025"
)

// NewServer is the constructor for the smtp agent
func NewServer(domain string, opts ...Option) agents.Agent {

	s := &server{
		listenAddr: defaultListenAddr,
		domain:     domain,
	}

	// Loop through each option
	for _, opt := range opts {
		opt(s)
	}

	return s
}

func WithListenAddr(addr string) Option {
	return func(h *server) {
		h.listenAddr = addr
	}
}

func (s *server) Start() {
	if s.instance != nil {
		log.Fatalf("Can't start a running server")
	}

	log.Printf("Starting smtp server @ %s ", s.listenAddr)

	instance := smtp.NewServer(&backend{})
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

	log.Printf("Started smtp server @ %s ", s.listenAddr)
}

func (s *server) Stop() error {
	if s.instance == nil {
		return errors.New("can't stop, server not running")
	}
	log.Printf("Stopping smtp server")

	err := s.instance.Close()
	s.instance = nil
	return err
}
