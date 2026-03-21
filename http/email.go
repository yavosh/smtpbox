package http

import (
	"errors"
	"net/http"

	"github.com/yavosh/smtpbox"
)

func (s *Server) handleGetEmails(w http.ResponseWriter, req *http.Request) {
	mailboxName := req.PathValue("mb")
	emails, err := s.emailService.List(mailboxName)
	if err != nil {
		if errors.Is(err, smtpbox.ErrNotFound) {
			jsonNotFound(w)
			return
		}
		s.log.Error("error listing emails", "mailbox", mailboxName, "error", err)
		jsonServerError(w)
		return
	}

	jsonOK(w, emails)
}
