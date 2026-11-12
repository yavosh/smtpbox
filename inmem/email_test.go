package inmem

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/yavosh/smtpbox"
)

func snapshotTestEmail() smtpbox.Email {
	return smtpbox.Email{
		From:     "sender@nar.cy",
		To:       []string{"recipient@nar.cy"},
		Body:     "original message",
		Received: time.Date(2026, time.November, 12, 10, 0, 0, 0, time.UTC),
	}
}

func TestStoreCopiesRecipients(t *testing.T) {
	store := NewEmailService()
	message := snapshotTestEmail()
	if err := store.Store("recipient@nar.cy", message); err != nil {
		t.Fatal(err)
	}
	message.To[0] = "changed@nar.cy"
	emails, err := store.List("recipient@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(emails, []smtpbox.Email{snapshotTestEmail()}) {
		t.Errorf("input mutation changed stored email: %v", emails)
	}
}

func TestListReturnsIndependentSnapshots(t *testing.T) {
	store := NewEmailService()
	message := snapshotTestEmail()
	for _, mailbox := range []string{"first@nar.cy", "second@nar.cy"} {
		if err := store.Store(mailbox, message); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.List("first@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.List("first@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	first[0].From = "changed@nar.cy"
	first[0].To[0] = "changed@nar.cy"
	first[0].Body = "changed body"
	first[0].Received = first[0].Received.Add(time.Hour)
	if !reflect.DeepEqual(second, []smtpbox.Email{snapshotTestEmail()}) {
		t.Errorf("separate snapshots share mutable data: %v", second)
	}
	for _, mailbox := range []string{"first@nar.cy", "second@nar.cy"} {
		emails, err := store.List(mailbox)
		if err != nil || !reflect.DeepEqual(emails, []smtpbox.Email{snapshotTestEmail()}) {
			t.Errorf("snapshot mutation changed %s: %v, %v", mailbox, emails, err)
		}
		metadata, err := store.GetMailbox(mailbox)
		if err != nil || metadata.Size != 1 || metadata.CreatedAt != message.Received {
			t.Errorf("snapshot mutation changed mailbox metadata: %v, %v", metadata, err)
		}
	}
}

func TestListPreservesRecipientShape(t *testing.T) {
	for _, recipients := range [][]string{nil, {}, {"recipient@nar.cy"}} {
		store := NewEmailService()
		message := snapshotTestEmail()
		message.To = recipients
		if err := store.Store("recipient@nar.cy", message); err != nil {
			t.Fatal(err)
		}
		emails, err := store.List("recipient@nar.cy")
		if err != nil {
			t.Fatal(err)
		}
		if len(emails) != 1 || !reflect.DeepEqual(emails[0].To, recipients) {
			t.Errorf("recipient shape changed: expected %#v, got %v", recipients, emails)
		}
	}
}

func TestSnapshotAppendCannotChangeStoredEmails(t *testing.T) {
	for _, count := range []int{1, 2, 3, 7} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store := NewEmailService()
			for range count {
				if err := store.Store("recipient@nar.cy", snapshotTestEmail()); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := store.List("recipient@nar.cy")
			if err != nil {
				t.Fatal(err)
			}
			latest := snapshotTestEmail()
			latest.Body = "later stored message"
			if err := store.Store("recipient@nar.cy", latest); err != nil {
				t.Fatal(err)
			}
			_ = append(snapshot, smtpbox.Email{Body: "caller append"})
			emails, err := store.List("recipient@nar.cy")
			if err != nil || len(emails) != count+1 || !reflect.DeepEqual(emails[count], latest) {
				t.Errorf("snapshot append changed later stored email: %v, %v", emails, err)
			}
		})
	}
}

func TestSnapshotRemainsStableAfterStore(t *testing.T) {
	store := NewEmailService()
	first := snapshotTestEmail()
	if err := store.Store("recipient@nar.cy", first); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.List("recipient@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	second := snapshotTestEmail()
	second.Body = "second message"
	second.Received = second.Received.Add(time.Hour)
	if err := store.Store("recipient@nar.cy", second); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, []smtpbox.Email{first}) {
		t.Errorf("later storage changed snapshot: %v", snapshot)
	}
	emails, err := store.List("recipient@nar.cy")
	if err != nil || !reflect.DeepEqual(emails, []smtpbox.Email{first, second}) {
		t.Errorf("stored ordering changed: %v, %v", emails, err)
	}
}

func TestConcurrentSnapshotMutation(t *testing.T) {
	store := NewEmailService()
	if err := store.Store("recipient@nar.cy", snapshotTestEmail()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.List("recipient@nar.cy")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for range 1000 {
			snapshot[0].Body = "caller mutation"
			snapshot[0].To[0] = "changed@nar.cy"
			snapshot[0].Received = time.Time{}
		}
	})
	wg.Go(func() {
		<-start
		for range 1000 {
			emails, err := store.List("recipient@nar.cy")
			if err != nil || !reflect.DeepEqual(emails, []smtpbox.Email{snapshotTestEmail()}) {
				t.Error("snapshot mutation changed concurrently read storage")
				return
			}
		}
	})
	close(start)
	wg.Wait()
}
