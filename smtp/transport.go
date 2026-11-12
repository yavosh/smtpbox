package smtp

import (
	"io"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox"
)

var _ smtp.Backend = &backend{}

type backend struct {
	server *Server
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{
		server: b.server,
	}, nil
}

// session implements smtp.Session and smtp.AuthSession
type session struct {
	server *Server
	from   string
	rcpt   []string
}

func (s *session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		s.server.log.Info("auth attempt", "username", username)
		return nil // accept all credentials — dev/testing tool
	}), nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	s.server.log.Info("mail from", "from", from)
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.server.log.Info("rcpt to", "to", to)
	at := strings.LastIndexByte(to, '@')
	if at <= 0 || at == len(to)-1 || !strings.EqualFold(to[at+1:], s.server.domain) {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 2},
			Message:      "Recipient domain not accepted",
		}
	}
	s.rcpt = append(s.rcpt, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	eml := smtpbox.Email{
		From:     s.from,
		To:       s.rcpt,
		Body:     string(b),
		Received: time.Now().UTC(),
	}

	for _, mailbox := range s.rcpt {
		if err := s.server.emailService.Store(mailbox, eml); err != nil {
			s.server.log.Error("error storing email", "mailbox", mailbox, "error", err)
			return &smtp.SMTPError{
				Code:         451,
				EnhancedCode: smtp.EnhancedCode{4, 3, 0},
				Message:      "Temporary storage failure",
			}
		}
		s.server.log.Info("email stored", "mailbox", mailbox, "from", eml.From)
	}

	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.rcpt = nil
}

func (s *session) Logout() error {
	s.server.log.Info("session ended")
	return nil
}
