package smtp

import (
	"errors"
	"github.com/yavosh/smtpbox"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
)

// backend implements SMTP server methods
type backend struct {
	server *Server
}

// session is returned after successful login
type session struct {
	server   *Server
	username string
	from     string
	rcpt     []string
}

// Login handles a login command with username and password.
func (b *backend) Login(_ *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	if password != "password" {
		return nil, errors.New("invalid username or password")
	}

	return &session{
		server:   b.server,
		username: username,
		rcpt:     make([]string, 0),
	}, nil
}

// AnonymousLogin requires clients to authenticate using SMTP AUTH before sending emails
func (b *backend) AnonymousLogin(_ *smtp.ConnectionState) (smtp.Session, error) {
	return nil, smtp.ErrAuthRequired
}

func (s *session) Mail(from string, _ smtp.MailOptions) error {
	s.server.log.Printf("Mail from: %s", from)
	s.from = from
	return nil
}

func (s *session) Rcpt(to string) error {
	s.server.log.Printf("Rcpt to: %s", to)
	s.rcpt = append(s.rcpt, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	// Read upto the limit
	r = io.LimitReader(r, 20480) // 20kb

	if b, err := io.ReadAll(r); err != nil {
		return err
	} else {

		// store email
		eml := smtpbox.Email{
			From:     s.from,
			To:       s.rcpt,
			Body:     string(b),
			Received: time.Now().UTC(),
		}

		for _, mailbox := range s.rcpt {
			if !strings.HasSuffix(mailbox, s.server.domain) {
				s.server.log.Printf("unsupported domain: %s", mailbox)
				continue
			}

			err := s.server.emailService.Store(mailbox, eml)
			s.server.log.Printf("Adding email to backend mb:%s email:%+v", mailbox, eml)
			if err != nil {
				s.server.log.Printf("Error storing email mb:%s", mailbox)
			}
		}
	}

	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.rcpt = make([]string, 0)
}

func (s *session) Logout() error {
	s.server.log.Print("session ended")
	return nil
}
