package smtp_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox/inmem"
	s "github.com/yavosh/smtpbox/smtp"
)

func TestSendEmail(t *testing.T) {
	backend := inmem.NewEmailService()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	localServer := s.NewServer("127.0.0.1:0", "example.net", backend)
	if err := localServer.StartContext(ctx); err != nil {
		t.Fatalf("could not start server: %v", err)
	}
	t.Cleanup(func() { localServer.Stop() })

	to := []string{"recipient@example.net"}
	msg := strings.NewReader("To: recipient@example.net\r\n" +
		"Subject: discount Gophers!\r\n" +
		"\r\n" +
		"This is the email body.\r\n")

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", localServer.Addr().String())
	if err != nil {
		t.Fatalf("could not dial %v", err)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	smtpClient := smtp.NewClient(conn)
	t.Cleanup(func() {
		stop()
		smtpClient.Close()
	})

	if err := smtpClient.Hello("localhost"); err != nil {
		t.Fatalf("could not hello %v", err)
	}

	if err := smtpClient.SendMail("sender@example.net", to, msg); err != nil {
		t.Fatalf("could not send %v", err)
	}

	if err := localServer.Stop(); err != nil {
		t.Fatalf("could not stop server %v", err)
	}

	mb, err := backend.GetMailbox("recipient@example.net")
	if err != nil {
		t.Fatalf("error getting mailbox: %v", err)
	}

	if mb.Addr != "recipient@example.net" {
		t.Errorf("expected mailbox addr recipient@example.net, got %s", mb.Addr)
	}
	if mb.Size != 1 {
		t.Errorf("expected mailbox size 1, got %d", mb.Size)
	}

	t.Logf("mailbox: %+v", mb)

	emails, err := backend.List("recipient@example.net")
	if err != nil {
		t.Fatalf("error listing emails: %v", err)
	}

	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}

	email := emails[0]
	if email.From != "sender@example.net" {
		t.Errorf("expected from sender@example.net, got %s", email.From)
	}
	if len(email.To) != 1 || email.To[0] != "recipient@example.net" {
		t.Errorf("expected to [recipient@example.net], got %v", email.To)
	}
	if !strings.Contains(email.Body, "This is the email body.") {
		t.Errorf("expected body to contain 'This is the email body.', got %s", email.Body)
	}

	t.Logf("email: %+v", email)
}
