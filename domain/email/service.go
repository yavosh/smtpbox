package email

type Service interface {
	Store(mailbox string, email Email) error
	AllMailboxes() ([]string, error)
	GetMailbox(mailbox string) (Mailbox, error)
	List(mailbox string) ([]Email, error)
}
