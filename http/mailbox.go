package http

import (
	"github.com/gorilla/mux"
	"log"
	"net/http"
)

func (s *Server) handleGetMailbox(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mailboxName := vars["mb"]
	mb, err := s.emailService.GetMailbox(mailboxName)

	if err != nil {
		Server500(w)
		log.Printf("Error %v", err)
		return
	}

	JSONResponse(w, http.StatusOK, map[string]interface{}{"status": "ok", "result": mb})
}
