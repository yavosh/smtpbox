package inmem

import (
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/yavosh/smtpbox"
)

// DefaultMaxMessages and DefaultMaxMemoryBytes bound retained storage.
const (
	DefaultMaxMessages          = 1000
	DefaultMaxMemoryBytes int64 = 64 << 20
)

type storedEmail struct {
	mailbox string
	email   smtpbox.Email
	bytes   int64
}

type EmailBackend struct {
	mu             sync.Mutex
	ring           []storedEmail
	head           int
	count          int
	usedBytes      int64
	fixedBytes     int64
	maxMemoryBytes int64
	log            *slog.Logger
}

func NewEmailService() smtpbox.EmailStoreBackend {
	store, err := NewEmailServiceWithLimits(DefaultMaxMessages, DefaultMaxMemoryBytes)
	if err != nil {
		panic(err)
	}
	return store
}

// NewEmailServiceWithLimits creates a global FIFO store for recipient copies.
func NewEmailServiceWithLimits(maxMessages int, maxMemoryBytes int64) (smtpbox.EmailStoreBackend, error) {
	if maxMessages <= 0 || maxMemoryBytes <= 0 {
		return nil, errors.New("storage limits must be positive")
	}
	slotBytes := int64(unsafe.Sizeof(storedEmail{}))
	fixedBytes := int64(unsafe.Sizeof(EmailBackend{}))
	if int64(maxMessages) > int64(int(^uint(0)>>1))/slotBytes {
		return nil, errors.New("message capacity exceeds addressable memory")
	}
	if maxMemoryBytes < fixedBytes || int64(maxMessages) > (maxMemoryBytes-fixedBytes)/slotBytes {
		return nil, errors.New("memory limit is too small for the message capacity")
	}
	fixedBytes += int64(maxMessages) * slotBytes
	return &EmailBackend{
		ring:           make([]storedEmail, maxMessages),
		usedBytes:      fixedBytes,
		fixedBytes:     fixedBytes,
		maxMemoryBytes: maxMemoryBytes,
		log:            slog.Default(),
	}, nil
}

func emailMemoryBytes(mailbox string, email smtpbox.Email, limit int64) (int64, bool) {
	var size int64
	addString := func(value string) bool {
		if int64(len(value)) > limit-size {
			return false
		}
		size += int64(len(value))
		return true
	}
	if !addString(mailbox) || !addString(email.From) || !addString(email.Body) {
		return 0, false
	}
	// The Email slice header is already included in the fixed ring slots.
	stringBytes := int64(unsafe.Sizeof(""))
	if int64(len(email.To)) > (limit-size)/stringBytes {
		return 0, false
	}
	size += int64(len(email.To)) * stringBytes
	for _, recipient := range email.To {
		if !addString(recipient) {
			return 0, false
		}
	}
	return size, true
}

func (s *EmailBackend) Store(mailbox string, eml smtpbox.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.log.Info("storing email", "mailbox", mailbox, "from", eml.From)
	size, fits := emailMemoryBytes(mailbox, eml, s.maxMemoryBytes-s.fixedBytes)
	if !fits {
		return errors.New("email exceeds storage memory limit")
	}
	for s.count == len(s.ring) || size > s.maxMemoryBytes-s.usedBytes {
		oldest := &s.ring[s.head]
		s.usedBytes -= oldest.bytes
		*oldest = storedEmail{}
		s.head = (s.head + 1) % len(s.ring)
		s.count--
	}

	mailbox = strings.Clone(mailbox)
	eml.From = strings.Clone(eml.From)
	eml.Body = strings.Clone(eml.Body)
	if eml.To != nil {
		recipients := make([]string, len(eml.To))
		for i, recipient := range eml.To {
			recipients[i] = strings.Clone(recipient)
		}
		eml.To = recipients
	}
	index := (s.head + s.count) % len(s.ring)
	s.ring[index] = storedEmail{mailbox: mailbox, email: eml, bytes: size}
	s.count++
	s.usedBytes += size
	return nil
}

func (s *EmailBackend) AllMailboxes() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]string, 0, s.count)
	seen := make(map[string]struct{}, s.count)
	for i := range s.count {
		mailbox := s.ring[(s.head+i)%len(s.ring)].mailbox
		if _, found := seen[mailbox]; !found {
			seen[mailbox] = struct{}{}
			list = append(list, mailbox)
		}
	}

	return list, nil
}

func (s *EmailBackend) GetMailbox(mailbox string) (smtpbox.Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata := smtpbox.Mailbox{Addr: mailbox}
	for i := range s.count {
		record := s.ring[(s.head+i)%len(s.ring)]
		if record.mailbox == mailbox {
			if metadata.Size == 0 {
				metadata.CreatedAt = record.email.Received
			}
			metadata.Size++
		}
	}
	if metadata.Size == 0 {
		return smtpbox.Mailbox{}, smtpbox.ErrNotFound
	}
	return metadata, nil
}

func (s *EmailBackend) List(mailbox string) ([]smtpbox.Email, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]smtpbox.Email, 0)
	for i := range s.count {
		record := s.ring[(s.head+i)%len(s.ring)]
		if record.mailbox == mailbox {
			email := record.email
			email.To = slices.Clone(email.To)
			list = append(list, email)
		}
	}
	if len(list) == 0 {
		return list, smtpbox.ErrNotFound
	}
	return list, nil
}
