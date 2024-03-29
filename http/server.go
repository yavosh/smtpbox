package http

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/yavosh/smtpbox"

	"github.com/emersion/go-smtp"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Server struct {
	port         int
	server       *http.Server
	log          smtp.Logger
	emailService smtpbox.EmailStoreBackend
}

func NewServer(port int, emailService smtpbox.EmailStoreBackend) *Server {
	router := mux.NewRouter().StrictSlash(true)

	s := &Server{
		port:         port,
		server:       &http.Server{Handler: router},
		emailService: emailService,
		log:          log.New(os.Stdout, "http ", log.LstdFlags),
	}

	// Prevent crashes
	router.Use(handlers.RecoveryHandler())

	router.HandleFunc("/", s.handleIndex).Methods(http.MethodGet)
	router.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet)
	router.HandleFunc("/v1/mailboxes", s.handleListMailboxes).Methods(http.MethodGet)
	router.HandleFunc("/v1/mailboxes/{mb}", s.handleGetMailbox).Methods(http.MethodGet)
	router.HandleFunc("/v1/mailboxes/{mb}/items", s.handleGetEmails).Methods(http.MethodGet)
	return s
}

func (s *Server) Start() error {
	s.log.Printf("Starting http server @ %d ", s.port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("http serve: %w", err)
	}

	go func() {
		// Ignore if due to Shutdown.
		if err := s.server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			s.log.Printf("Error starting http server %v", err)
		}
	}()

	s.log.Printf("Started http server @ %d ", s.port)
	return nil
}

func (s *Server) Stop() error {
	if s.server == nil {
		return errors.New("can't stop, server not running")
	}
	s.log.Printf("Stopping http server")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err := s.server.Shutdown(ctx)
	s.log.Printf("Stopped http server")
	return err
}

func (s *Server) handleIndex(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}

	JSONResponse(w, http.StatusOK, map[string]string{"name": "smtpbox api service"})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
