package inmem

import (
	"log/slog"
	"slices"
	"sync"

	"github.com/yavosh/smtpbox"
)

type EmailBackend struct {
	mu    sync.Mutex
	store map[string][]smtpbox.Email
	log   *slog.Logger
}

func NewEmailService() smtpbox.EmailStoreBackend {
	return &EmailBackend{
		store: make(map[string][]smtpbox.Email),
		log:   slog.Default(),
	}
}

func (s *EmailBackend) Store(mailbox string, eml smtpbox.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.log.Info("storing email", "mailbox", mailbox, "from", eml.From)
	if _, found := s.store[mailbox]; !found {
		s.store[mailbox] = []smtpbox.Email{}
	}

	eml.To = slices.Clone(eml.To)
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
		return smtpbox.Mailbox{}, smtpbox.ErrNotFound
	}

	return smtpbox.Mailbox{
		Addr:      mailbox,
		Size:      len(emails),
		CreatedAt: emails[0].Received,
	}, nil
}

func (s *EmailBackend) List(mailbox string) ([]smtpbox.Email, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	emails, found := s.store[mailbox]
	if !found || len(emails) == 0 {
		return []smtpbox.Email{}, smtpbox.ErrNotFound
	}

	list := slices.Clone(emails)
	for i := range list {
		list[i].To = slices.Clone(list[i].To)
	}
	return list, nil
}
