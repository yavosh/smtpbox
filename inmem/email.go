package inmem

import (
	"log"
	"sync"

	"github.com/yavosh/smtpbox"
)

type EmailBackend struct {
	mu    sync.Mutex
	store map[string][]smtpbox.Email
}

// NewEmailService is the constructor for in-mem service
func NewEmailService() smtpbox.EmailStoreBackend {
	return &EmailBackend{store: make(map[string][]smtpbox.Email, 0)}
}

func (s *EmailBackend) Store(mailbox string, eml smtpbox.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Println("Storing email", mailbox, eml)
	_, found := s.store[mailbox]
	if !found {
		s.store[mailbox] = make([]smtpbox.Email, 0)
	}

	s.store[mailbox] = append(s.store[mailbox], eml)
	return nil
}

func (s *EmailBackend) AllMailboxes() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]string, 0, len(s.store))
	for key := range s.store {
		list = append(list, key)
	}

	return list, nil
}

func (s *EmailBackend) GetMailbox(mailbox string) (smtpbox.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	emails, found := s.store[mailbox]
	if !found || len(emails) == 0 {
		return smtpbox.Mailbox{}, smtpbox.ErrorNotFound
	}

	firstEmail := emails[0]
	mb := smtpbox.Mailbox{
		Addr:      mailbox,
		Size:      len(emails),
		CreatedAt: firstEmail.Received,
	}

	return mb, nil
}

func (s *EmailBackend) List(mailbox string) ([]smtpbox.Email, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	emails, found := s.store[mailbox]
	if !found || len(emails) == 0 {
		return []smtpbox.Email{}, smtpbox.ErrorNotFound
	}

	return emails, nil
}
