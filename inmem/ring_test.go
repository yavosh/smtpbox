package inmem

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/yavosh/smtpbox"
)

func quietStore(store smtpbox.EmailStoreBackend) {
	store.(*EmailBackend).log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDefaultStoreBoundsCount(t *testing.T) {
	store := NewEmailService()
	quietStore(store)
	for i := range 1250 {
		if err := store.Store(fmt.Sprintf("recipient-%d@nar.cy", i), snapshotTestEmail()); err != nil {
			t.Fatal(err)
		}
	}
	mailboxes, err := store.AllMailboxes()
	if err != nil {
		t.Fatal(err)
	}
	if len(mailboxes) > 1000 {
		t.Errorf("default store retained %d records; expected at most 1000", len(mailboxes))
	}
}

func TestDefaultStoreBoundsBytes(t *testing.T) {
	store := NewEmailService()
	quietStore(store)
	for i := range 70 {
		message := snapshotTestEmail()
		message.Body = fmt.Sprintf("%06d", i) + strings.Repeat("x", (1<<20)-6)
		if err := store.Store("recipient@nar.cy", message); err != nil {
			t.Fatal(err)
		}
	}
	emails, err := store.List("recipient@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	var bodyBytes int64
	for _, email := range emails {
		bodyBytes += int64(len(email.Body))
	}
	if bodyBytes > 64<<20 {
		t.Errorf("default store retained %d body bytes; expected at most %d", bodyBytes, 64<<20)
	}
}

func fixedRingMemory(capacity int) int64 {
	return int64(unsafe.Sizeof(EmailBackend{})) + int64(capacity)*int64(unsafe.Sizeof(storedEmail{}))
}

func recordMemory(mailbox string, email smtpbox.Email) int64 {
	size := int64(len(mailbox)+len(email.From)+len(email.Body)) + int64(len(email.To))*int64(unsafe.Sizeof(""))
	for _, recipient := range email.To {
		size += int64(len(recipient))
	}
	return size
}

func newRingTestStore(t *testing.T, capacity int, budget int64) *EmailBackend {
	t.Helper()
	store, err := NewEmailServiceWithLimits(capacity, budget)
	if err != nil {
		t.Fatal(err)
	}
	quietStore(store)
	return store.(*EmailBackend)
}

func assertRingAccounting(t *testing.T, store *EmailBackend) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	active := make(map[int]bool, store.count)
	wantBytes := fixedRingMemory(len(store.ring))
	for i := range store.count {
		index := (store.head + i) % len(store.ring)
		active[index] = true
		record := store.ring[index]
		want := recordMemory(record.mailbox, record.email)
		if record.bytes != want {
			t.Errorf("record costs %d bytes, expected %d", record.bytes, want)
		}
		wantBytes += want
	}
	for index, record := range store.ring {
		if !active[index] && !reflect.DeepEqual(record, storedEmail{}) {
			t.Errorf("inactive slot %d retains message references", index)
		}
	}
	if store.count > len(store.ring) || store.usedBytes != wantBytes || store.usedBytes > store.maxMemoryBytes {
		t.Errorf("incorrect accounting: count=%d capacity=%d bytes=%d expected=%d budget=%d", store.count, len(store.ring), store.usedBytes, wantBytes, store.maxMemoryBytes)
	}
}

func TestRingValidatesLimits(t *testing.T) {
	for _, tc := range []struct {
		count  int
		budget int64
	}{
		{0, 1024},
		{-1, 1024},
		{1, 0},
		{1, -1},
		{2, fixedRingMemory(2) - 1},
		{int(^uint(0) >> 1), math.MaxInt64},
	} {
		if store, err := NewEmailServiceWithLimits(tc.count, tc.budget); err == nil || store != nil {
			t.Errorf("expected invalid limits rejected: count=%d budget=%d", tc.count, tc.budget)
		}
	}
	store := newRingTestStore(t, 1, fixedRingMemory(1))
	if err := store.Store("", smtpbox.Email{}); err != nil {
		t.Fatalf("record with no dynamic bytes must fit exact fixed budget: %v", err)
	}
	assertRingAccounting(t, store)
}

func TestRingGlobalFIFOAndMetadata(t *testing.T) {
	store := newRingTestStore(t, 3, 1<<20)
	if mailboxes, err := store.AllMailboxes(); err != nil || mailboxes == nil || len(mailboxes) != 0 {
		t.Fatalf("empty mailbox list must be nonnil: %v, %v", mailboxes, err)
	}
	var secondA smtpbox.Email
	for i, mailbox := range []string{"a@nar.cy", "b@nar.cy", "a@nar.cy", "c@nar.cy", "d@nar.cy"} {
		message := snapshotTestEmail()
		message.Body = fmt.Sprint(i)
		message.Received = message.Received.Add(time.Duration(i) * time.Hour)
		if i == 2 {
			secondA = message
		}
		if err := store.Store(mailbox, message); err != nil {
			t.Fatal(err)
		}
		assertRingAccounting(t, store)
	}
	emails, err := store.List("a@nar.cy")
	if err != nil || !reflect.DeepEqual(emails, []smtpbox.Email{secondA}) {
		t.Fatalf("incorrect retained a mailbox: %v, %v", emails, err)
	}
	metadata, err := store.GetMailbox("a@nar.cy")
	if err != nil || metadata.Size != 1 || metadata.CreatedAt != secondA.Received {
		t.Errorf("metadata must describe oldest retained record: %v, %v", metadata, err)
	}
	if _, err := store.List("b@nar.cy"); !errors.Is(err, smtpbox.ErrNotFound) {
		t.Errorf("evicted mailbox must disappear: %v", err)
	}
	mailboxes, err := store.AllMailboxes()
	if err != nil || !reflect.DeepEqual(mailboxes, []string{"a@nar.cy", "c@nar.cy", "d@nar.cy"}) {
		t.Errorf("incorrect retained mailboxes: %v, %v", mailboxes, err)
	}
	for i := range 200 {
		if err := store.Store(fmt.Sprintf("new-%d@nar.cy", i), snapshotTestEmail()); err != nil {
			t.Fatal(err)
		}
	}
	mailboxes, err = store.AllMailboxes()
	if err != nil || !reflect.DeepEqual(mailboxes, []string{"new-197@nar.cy", "new-198@nar.cy", "new-199@nar.cy"}) {
		t.Errorf("historical mailbox addresses remain: %v, %v", mailboxes, err)
	}
	if _, err := store.GetMailbox("a@nar.cy"); !errors.Is(err, smtpbox.ErrNotFound) {
		t.Errorf("evicted metadata remains: %v", err)
	}
	assertRingAccounting(t, store)
}

func TestRingByteLimitEvictsMultipleRecords(t *testing.T) {
	message := snapshotTestEmail()
	message.Body = "x"
	message.To = []string{"old-0@nar.cy"}
	smallBytes := recordMemory("old-0@nar.cy", message)
	store := newRingTestStore(t, 5, fixedRingMemory(5)+3*smallBytes)
	for i := range 3 {
		mailbox := fmt.Sprintf("old-%d@nar.cy", i)
		message.To = []string{mailbox}
		if err := store.Store(mailbox, message); err != nil {
			t.Fatal(err)
		}
	}
	message.To = []string{"new-0@nar.cy"}
	message.Body = ""
	message.Body = strings.Repeat("x", int(3*smallBytes-recordMemory("new-0@nar.cy", message)-1))
	if err := store.Store("new-0@nar.cy", message); err != nil {
		t.Fatal(err)
	}
	mailboxes, err := store.AllMailboxes()
	if err != nil || !reflect.DeepEqual(mailboxes, []string{"new-0@nar.cy"}) {
		t.Errorf("byte limit did not evict oldest records globally: %v, %v", mailboxes, err)
	}
	if store.count != 1 {
		t.Errorf("expected three evictions, retained %d records", store.count)
	}
	assertRingAccounting(t, store)
}

func TestRingExactBudgetAndOversizePreservesRecords(t *testing.T) {
	first := snapshotTestEmail()
	second := snapshotTestEmail()
	second.Body = "second"
	budget := fixedRingMemory(2) + recordMemory("first@nar.cy", first) + recordMemory("second@nar.cy", second)
	store := newRingTestStore(t, 2, budget)
	if err := store.Store("first@nar.cy", first); err != nil {
		t.Fatal(err)
	}
	if err := store.Store("second@nar.cy", second); err != nil {
		t.Fatal(err)
	}
	if store.usedBytes != budget {
		t.Fatalf("expected exact budget use, got %d of %d", store.usedBytes, budget)
	}
	beforeHead := store.head
	oversize := smtpbox.Email{Body: strings.Repeat("x", int(budget-store.fixedBytes+1))}
	if err := store.Store("", oversize); err == nil {
		t.Fatal("expected impossible record rejected")
	}
	if store.head != beforeHead || store.count != 2 || store.usedBytes != budget {
		t.Fatal("impossible record evicted existing data")
	}
	for mailbox, want := range map[string]smtpbox.Email{"first@nar.cy": first, "second@nar.cy": second} {
		emails, err := store.List(mailbox)
		if err != nil || !reflect.DeepEqual(emails, []smtpbox.Email{want}) {
			t.Errorf("rejection changed existing record: %v, %v", emails, err)
		}
	}
	exact := smtpbox.Email{Body: strings.Repeat("x", int(budget-store.fixedBytes))}
	if err := store.Store("", exact); err != nil {
		t.Fatalf("exact dynamic budget must fit: %v", err)
	}
	if store.count != 1 || store.usedBytes != budget {
		t.Errorf("incorrect exact-budget retention: count=%d bytes=%d", store.count, store.usedBytes)
	}
	assertRingAccounting(t, store)

	store = newRingTestStore(t, 1, fixedRingMemory(1)+2*int64(unsafe.Sizeof("")))
	if err := store.Store("", smtpbox.Email{To: []string{"", "", ""}}); err == nil {
		t.Error("recipient element headers must count toward the byte budget")
	}
	assertRingAccounting(t, store)
}

func TestRingOwnsBackingStorage(t *testing.T) {
	store := newRingTestStore(t, 2, 4096)
	backing := strings.Repeat("x", 1<<20)
	mailbox := backing[:12]
	message := smtpbox.Email{From: backing[12:24], Body: backing[24:40]}
	backingRecipients := make([]string, 1<<16)
	backingRecipients[0] = backing[40:56]
	message.To = backingRecipients[:1]
	if err := store.Store(mailbox, message); err != nil {
		t.Fatal(err)
	}
	record := store.ring[store.head]
	for _, pair := range [][2]string{
		{record.mailbox, mailbox},
		{record.email.From, message.From},
		{record.email.Body, message.Body},
		{record.email.To[0], message.To[0]},
	} {
		if pair[0] != pair[1] || unsafe.StringData(pair[0]) == unsafe.StringData(pair[1]) {
			t.Error("stored string retained external backing storage")
		}
	}
	if unsafe.SliceData(record.email.To) == unsafe.SliceData(message.To) {
		t.Error("stored recipients retained external backing array")
	}
	message.To = backingRecipients[:0]
	if err := store.Store("empty@nar.cy", message); err != nil {
		t.Fatal(err)
	}
	record = store.ring[(store.head+1)%len(store.ring)]
	if record.email.To == nil || len(record.email.To) != 0 || unsafe.SliceData(record.email.To) == unsafe.SliceData(message.To) {
		t.Error("empty recipients lost shape or retained external backing array")
	}
	assertRingAccounting(t, store)
}

func TestRingNormalizesReceivedToUTC(t *testing.T) {
	store := newRingTestStore(t, 1, 1024)
	location := time.FixedZone(strings.Repeat("zone", 1<<18), -(3*60+30)*60)
	message := snapshotTestEmail()
	message.Received = time.Date(2026, time.November, 12, 10, 30, 0, 123, location)
	original := message.Received
	if err := store.Store("recipient@nar.cy", message); err != nil {
		t.Fatal(err)
	}
	emails, err := store.List("recipient@nar.cy")
	if err != nil || len(emails) != 1 {
		t.Fatalf("expected one stored message: count=%d error=%v", len(emails), err)
	}
	if emails[0].Received.Location() != time.UTC {
		t.Error("stored timestamp retains the caller's time zone")
	}
	if !emails[0].Received.Equal(original) {
		t.Error("timestamp normalization changed the received instant")
	}
	metadata, err := store.GetMailbox("recipient@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.CreatedAt.Location() != time.UTC || !metadata.CreatedAt.Equal(original) {
		t.Error("mailbox timestamp must retain the instant in UTC")
	}
	if message.Received != original || message.Received.Location() != location {
		t.Error("storage changed the caller's timestamp")
	}
	assertRingAccounting(t, store)
}

func TestRingConcurrentOperations(t *testing.T) {
	store := newRingTestStore(t, 32, 64<<10)
	var wg sync.WaitGroup
	errorsFound := make(chan error, 8)
	for worker := range 4 {
		wg.Go(func() {
			for range 200 {
				if err := store.Store(fmt.Sprintf("worker-%d@nar.cy", worker), snapshotTestEmail()); err != nil {
					errorsFound <- err
					return
				}
			}
		})
		wg.Go(func() {
			for range 200 {
				mailboxes, err := store.AllMailboxes()
				if err != nil {
					errorsFound <- err
					return
				}
				for _, mailbox := range mailboxes {
					if _, err := store.GetMailbox(mailbox); err != nil && !errors.Is(err, smtpbox.ErrNotFound) {
						errorsFound <- err
						return
					}
					emails, err := store.List(mailbox)
					if err != nil && !errors.Is(err, smtpbox.ErrNotFound) {
						errorsFound <- err
						return
					}
					for i := range emails {
						if emails[i].Body != "original message" {
							errorsFound <- fmt.Errorf("snapshot mutation changed retained message")
							return
						}
						emails[i].Body = "caller mutation"
						emails[i].To[0] = "changed@nar.cy"
					}
				}
			}
		})
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	assertRingAccounting(t, store)
}
