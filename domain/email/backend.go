package email

type Backend interface {
	Store(mailbox string, email Email) error
	AllMailboxes() ([]string, error)
	GetMailbox(mailbox string) (Mailbox, error)
	List(mailbox string) ([]Email, error)
}
