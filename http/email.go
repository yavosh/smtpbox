package http

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) handleGetEmails(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mailboxName := vars["mb"]
	emails, err := s.emailService.List(mailboxName)

	if err != nil {
		Server500(w)
		log.Printf("Error %v", err)
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{"status": "ok", "result": emails})
}
