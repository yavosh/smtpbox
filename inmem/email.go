package inmem

import (
	"log"
	"sync"

	"github.com/yavosh/smtpbox/domain"
	"github.com/yavosh/smtpbox/domain/email"
)

type EmailService struct {
	mu    sync.Mutex
	store map[string][]email.Email
}

// NewEmailService is the constructor for in-mem service
func NewEmailService() email.Service {
	return &EmailService{store: make(map[string][]email.Email, 0)}
}

func (s *EmailService) Store(mailbox string, eml email.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Println("Storing email", mailbox, eml)
	_, found := s.store[mailbox]
	if !found {
		s.store[mailbox] = make([]email.Email, 0)
	}

	s.store[mailbox] = append(s.store[mailbox], eml)
	return nil
}

func (s *EmailService) GetMailbox(mailbox string) (email.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	emails, found := s.store[mailbox]
	if !found || len(emails) == 0 {
		return email.Mailbox{}, domain.ErrorNotFound
	}

	firstEmail := emails[0]
	mb := email.Mailbox{
		Addr:      mailbox,
		Size:      len(emails),
		CreatedAt: firstEmail.Received,
	}

	return mb, nil
}

func (s *EmailService) List(mailbox string) ([]email.Email, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	emails, found := s.store[mailbox]
	if !found || len(emails) == 0 {
		return []email.Email{}, domain.ErrorNotFound
	}

	return emails, nil
}
