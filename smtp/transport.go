package smtp

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/yavosh/smtpbox"

	"github.com/emersion/go-smtp"
)

var _ smtp.Backend = &backend{}

// backend implements SMTP server methods
type backend struct {
	server *Server
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{
		server: b.server,
		rcpt:   make([]string, 0),
	}, nil
}

// session is returned after successful login
type session struct {
	server   *Server
	username string
	from     string
	rcpt     []string
}

func (s *session) AuthPlain(username, password string) error {
	if password != "password" {
		return errors.New("invalid username or password")
	}

	s.username = username
	return nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	s.server.log.Printf("Mail from: %s %#v", from, opts)
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.server.log.Printf("Rcpt to: %s %#v", to, opts)
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
