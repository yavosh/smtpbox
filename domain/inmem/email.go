package inmem

import "github.com/yavosh/smtpmocker/domain/email"

type EmailRepository struct {
	email.Repository
	store map[string][]email.Email
}

// NewEmailRepository is constructor for in-memory repository
func NewEmailRepository() *EmailRepository {
	return &EmailRepository{
		store: make(map[string][]email.Email, 0),
	}
}

func (r *EmailRepository) Store(addr string, eml email.Email) error {

	emails, found := r.store[addr]
	if !found {
		emails = make([]email.Email, 0)
	}

	emails = append(emails, eml)

	return nil
}
