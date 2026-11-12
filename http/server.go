package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/yavosh/smtpbox"
)

type Server struct {
	port         int
	listenHost   string
	mux          *http.ServeMux
	server       *http.Server
	auth         *basicAuth
	authErr      error
	log          *slog.Logger
	emailService smtpbox.EmailStoreBackend
	mu           sync.Mutex
	listener     net.Listener
	serveDone    chan struct{}
}

type Option func(*Server)

func WithListenHost(host string) Option {
	return func(s *Server) { s.listenHost = host }
}

func NewServer(port int, emailService smtpbox.EmailStoreBackend, opts ...Option) *Server {
	mux := http.NewServeMux()

	s := &Server{
		port:         port,
		listenHost:   "127.0.0.1",
		mux:          mux,
		emailService: emailService,
		log:          clogHTTP(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.server = &http.Server{
		Addr:              net.JoinHostPort(s.listenHost, strconv.Itoa(s.port)),
		Handler:           recoveryMiddleware(basicAuthMiddleware(s.auth, mux)),
		ReadHeaderTimeout: 5 * time.Second,
		// Route OPTIONS * through authentication instead of the automatic response.
		DisableGeneralOptionsHandler: s.auth != nil,
	}

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/mailboxes", s.handleListMailboxes)
	mux.HandleFunc("GET /v1/mailboxes/{mb}", s.handleGetMailbox)
	mux.HandleFunc("GET /v1/mailboxes/{mb}/items", s.handleGetEmails)
	return s
}

func (s *Server) Start() error {
	return s.StartContext(context.Background())
}

// StartContext binds the listener before starting the HTTP server.
func (s *Server) StartContext(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listenHost == "" {
		return errors.New("listen host must not be empty")
	}
	if s.authErr != nil {
		return s.authErr
	}
	if s.serveDone != nil {
		return errors.New("http server already started")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	s.log.Info("starting http server", "port", s.port)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = ln.Close()
		return fmt.Errorf("http listen: %w", err)
	}
	s.listener = ln
	done := make(chan struct{})
	s.serveDone = done

	go func() {
		defer close(done)
		if err := s.server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("http server error", "error", err)
		}
	}()

	s.log.Info("started http server", "port", s.port)
	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	ln, done := s.listener, s.serveDone
	s.mu.Unlock()

	if ln == nil {
		return nil
	}
	s.log.Info("stopping http server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, s.server.Close())
	}
	// Shutdown can finish before Serve registers its listener.
	_ = ln.Close()
	<-done
	s.mu.Lock()
	s.listener = nil
	s.mu.Unlock()
	s.log.Info("stopped http server")
	return err
}

// Addr returns the bound address, or nil before startup or after Stop.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]string{"name": "smtpbox api service"})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				clogHTTP().Error("panic recovered", "error", err, "stack", string(debug.Stack()))
				jsonServerError(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
