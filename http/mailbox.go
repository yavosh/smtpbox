package http

import (
	"errors"
	"net/http"

	"github.com/yavosh/smtpbox"
)

func (s *Server) handleListMailboxes(w http.ResponseWriter, _ *http.Request) {
	list, err := s.emailService.AllMailboxes()
	if err != nil {
		s.log.Error("error listing mailboxes", "error", err)
		jsonServerError(w)
		return
	}

	jsonOK(w, list)
}

func (s *Server) handleGetMailbox(w http.ResponseWriter, req *http.Request) {
	mailboxName := req.PathValue("mb")
	mb, err := s.emailService.GetMailbox(mailboxName)
	if err != nil {
		if errors.Is(err, smtpbox.ErrNotFound) {
			jsonNotFound(w)
			return
		}
		s.log.Error("error getting mailbox", "mailbox", mailboxName, "error", err)
		jsonServerError(w)
		return
	}

	jsonOK(w, mb)
}
