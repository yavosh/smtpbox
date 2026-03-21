package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/yavosh/smtpbox"
)

type Server struct {
	port         int
	mux          *http.ServeMux
	server       *http.Server
	log          *slog.Logger
	emailService smtpbox.EmailStoreBackend
}

func NewServer(port int, emailService smtpbox.EmailStoreBackend) *Server {
	mux := http.NewServeMux()

	s := &Server{
		port:         port,
		mux:          mux,
		server:       &http.Server{Handler: recoveryMiddleware(mux)},
		emailService: emailService,
		log:          slog.Default(),
	}

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/mailboxes", s.handleListMailboxes)
	mux.HandleFunc("GET /v1/mailboxes/{mb}", s.handleGetMailbox)
	mux.HandleFunc("GET /v1/mailboxes/{mb}/items", s.handleGetEmails)
	return s
}

func (s *Server) Start() error {
	s.log.Info("starting http server", "port", s.port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("http serve: %w", err)
	}

	go func() {
		if err := s.server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("http server error", "error", err)
		}
	}()

	s.log.Info("started http server", "port", s.port)
	return nil
}

func (s *Server) Stop() error {
	if s.server == nil {
		return errors.New("can't stop, server not running")
	}
	s.log.Info("stopping http server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	s.log.Info("stopped http server")
	return err
}

func (s *Server) handleIndex(w http.ResponseWriter, req *http.Request) {
	jsonOK(w, map[string]string{"name": "smtpbox api service"})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err, "stack", string(debug.Stack()))
				jsonServerError(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
