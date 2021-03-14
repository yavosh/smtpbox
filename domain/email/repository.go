package email

type Repository interface {
	Store(addr string, email Email) error
	GetMailbox(addr string) (Mailbox, error)
	List(addr string) ([]Email, error)
}
