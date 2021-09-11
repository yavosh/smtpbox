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

	"github.com/emersion/go-smtp"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/yavosh/smtpbox/domain/email"
)

type Server struct {
	port         int
	server       *http.Server
	log          smtp.Logger
	emailService email.Service
}

func NewServer(port int, emailService email.Service) *Server {
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
	router.HandleFunc("/v1/mailbox/{mb}", s.handleGetMailbox).Methods(http.MethodPost)
	router.HandleFunc("/v1/mailbox/{mb}/items", s.handleGetEmails).Methods(http.MethodPost)
	return s
}

func (s *Server) Start() error {
	log.Printf("Starting http server @ %d ", s.port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("http serve: %w", err)
	}

	go func() {
		// Ignore if due to Shutdown.
		if err := s.server.Serve(ln); err != http.ErrServerClosed {
			log.Fatalf("Error starting http server %v", err)
		}
	}()

	log.Printf("Started http server @ %d ", s.port)
	return nil
}

func (s *Server) Stop() error {
	if s.server == nil {
		return errors.New("can't stop, server not running")
	}
	log.Printf("Stopping http server")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err := s.server.Shutdown(ctx)
	log.Printf("Stopped http server")
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
