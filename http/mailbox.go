package http

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) handleListMailboxes(w http.ResponseWriter, req *http.Request) {
	list, err := s.emailService.AllMailboxes()
	if err != nil {
		log.Printf("Error %v", err)
		Server500(w)
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{"status": "ok", "result": list})
}

func (s *Server) handleGetMailbox(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mailboxName := vars["mb"]
	mb, err := s.emailService.GetMailbox(mailboxName)

	if err != nil {
		log.Printf("Error %v", err)
		Server500(w)
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{"status": "ok", "result": mb})
}
