package smtpbox

import "time"

type EmailStoreBackend interface {
	Store(mailbox string, email Email) error
	AllMailboxes() ([]string, error)
	GetMailbox(mailbox string) (Mailbox, error)
	List(mailbox string) ([]Email, error)
}

type Email struct {
	From     string
	To       []string
	Body     string
	Received time.Time
}

type Mailbox struct {
	Addr      string
	Size      int
	CreatedAt time.Time
}
