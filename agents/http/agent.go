package http

import (
	"context"
	"errors"
	"github.com/yavosh/smtpmocker/web"
	"log"
	"net/http"
	"time"

	"github.com/yavosh/smtpmocker/agents"
)

type server struct {
	listenAddr string
	instance   *http.Server
}

// Option is an option to the server
type Option func(*server)

const (
	defaultListenAddr = ":8080"
)

// NewServer is the constructor for the http agent
func NewServer(opts ...Option) agents.Agent {

	s := &server{
		listenAddr: defaultListenAddr,
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
	log.Printf("Starting http server @ %s ", s.listenAddr)
	instance := &http.Server{Addr: s.listenAddr}
	s.instance = instance

	go func() {
		if err := http.ListenAndServe(s.listenAddr, web.NewHttpHandler()); err != nil {
			panic(err)
		}
	}()

	log.Printf("Started smtp server @ %s ", s.listenAddr)

}

func (s *server) Stop() error {
	if s.instance == nil {
		return errors.New("can't stop, server not running")
	}
	log.Printf("Stopping http server")

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*100)
	defer cancel()

	err := s.instance.Shutdown(ctx)
	return err
}
