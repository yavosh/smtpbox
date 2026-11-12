package smtp

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox"
	"github.com/yavosh/smtpbox/inmem"
)

func newDeliveryTestClient(t *testing.T, domain string, store smtpbox.EmailStoreBackend) *gosmtp.Client {
	t.Helper()
	server := NewServer("127.0.0.1:0", domain, store)
	if err := server.StartContext(smtpTestContext(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	return newSMTPTestClient(t, smtpTestContext(t), server.Addr().String())
}

func smtpTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newSMTPTestClient(t *testing.T, ctx context.Context, address string) *gosmtp.Client {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		t.Fatalf("dial SMTP server: %v", err)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	client := gosmtp.NewClient(conn)
	t.Cleanup(func() {
		stop()
		client.Close()
	})
	return client
}

func TestSMTPRecipientDomain(t *testing.T) {
	for _, tc := range []struct {
		name      string
		domain    string
		recipient string
		mailbox   string
		accepted  bool
	}{
		{"exact", "nar.cy", "recipient@nar.cy", "recipient@nar.cy", true},
		{"recipient-case", "nar.cy", "recipient@NAR.CY", "recipient@NAR.CY", true},
		{"configured-case", "NAR.CY", "recipient@nar.cy", "recipient@nar.cy", true},
		{"quoted-local", "nar.cy", `"local@part"@nar.cy`, "local@part@nar.cy", true},
		{"unsupported", "nar.cy", "recipient@narity.com", "recipient@narity.com", false},
		{"suffix", "arity.com", "recipient@narity.com", "recipient@narity.com", false},
		{"subdomain", "cy", "recipient@nar.cy", "recipient@nar.cy", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := inmem.NewEmailService()
			client := newDeliveryTestClient(t, tc.domain, store)
			if err := client.Mail("sender@nar.cy", nil); err != nil {
				t.Fatal(err)
			}
			err := client.Rcpt(tc.recipient, nil)
			if !tc.accepted {
				if smtpErr, ok := errors.AsType[*gosmtp.SMTPError](err); !ok || smtpErr.Code != 550 {
					t.Fatalf("expected RCPT rejection 550, got %v", err)
				}
				if _, err := store.List(tc.mailbox); !errors.Is(err, smtpbox.ErrNotFound) {
					t.Fatalf("expected no stored message, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("accept recipient: %v", err)
			}
			data, err := client.Data()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.Write([]byte("Subject: accepted\r\n\r\nmessage\r\n")); err != nil {
				t.Fatal(err)
			}
			if err := data.Close(); err != nil {
				t.Fatal(err)
			}
			emails, err := store.List(tc.mailbox)
			if err != nil || len(emails) != 1 {
				t.Fatalf("expected one message in original mailbox, got %d, %v", len(emails), err)
			}
		})
	}
}

func TestSMTPRejectedRecipientAndReset(t *testing.T) {
	store := inmem.NewEmailService()
	client := newDeliveryTestClient(t, "nar.cy", store)
	if err := client.Mail("sender@nar.cy", nil); err != nil {
		t.Fatal(err)
	}
	accepted := []string{"first@nar.cy", "second@nar.cy"}
	if err := client.Rcpt(accepted[0], nil); err != nil {
		t.Fatal(err)
	}
	err := client.Rcpt("rejected@narity.com", nil)
	if smtpErr, ok := errors.AsType[*gosmtp.SMTPError](err); !ok || smtpErr.Code != 550 {
		t.Fatalf("expected RCPT rejection 550, got %v", err)
	}
	if err := client.Rcpt(accepted[1], nil); err != nil {
		t.Fatal(err)
	}
	data, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Write([]byte("Subject: accepted recipients\r\n\r\nmessage\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	for _, mailbox := range accepted {
		emails, err := store.List(mailbox)
		if err != nil || len(emails) != 1 {
			t.Fatalf("expected one message in %s, got %d, %v", mailbox, len(emails), err)
		}
		if !reflect.DeepEqual(emails[0].To, accepted) {
			t.Errorf("expected accepted recipients %v, got %v", accepted, emails[0].To)
		}
	}
	if _, err := store.List("rejected@narity.com"); !errors.Is(err, smtpbox.ErrNotFound) {
		t.Fatalf("expected rejected mailbox absent, got %v", err)
	}

	if err := client.Mail("sender@nar.cy", nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Rcpt("reset@nar.cy", nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := client.SendMail("sender@nar.cy", []string{"reused@nar.cy"}, strings.NewReader("Subject: reuse\r\n\r\nmessage\r\n")); err != nil {
		t.Fatal(err)
	}
	emails, err := store.List("reused@nar.cy")
	if err != nil || len(emails) != 1 {
		t.Fatalf("expected one message after reset, got %d, %v", len(emails), err)
	}
	if !reflect.DeepEqual(emails[0].To, []string{"reused@nar.cy"}) {
		t.Errorf("recipients leaked after reset: %v", emails[0].To)
	}
	if _, err := store.List("reset@nar.cy"); !errors.Is(err, smtpbox.ErrNotFound) {
		t.Fatalf("expected reset recipient absent, got %v", err)
	}
}

type failingEmailStore struct {
	smtpbox.EmailStoreBackend
	failMailbox string
}

func (s failingEmailStore) Store(mailbox string, email smtpbox.Email) error {
	if mailbox == s.failMailbox {
		return errors.New("private backend detail")
	}
	return s.EmailStoreBackend.Store(mailbox, email)
}

func TestSMTPStorageFailure(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "first-recipient"
		if partial {
			name = "later-recipient"
		}
		t.Run(name, func(t *testing.T) {
			store := inmem.NewEmailService()
			client := newDeliveryTestClient(t, "nar.cy", failingEmailStore{store, "failed@nar.cy"})
			recipients := []string{"failed@nar.cy"}
			if partial {
				recipients = []string{"stored@nar.cy", "failed@nar.cy", "unattempted@nar.cy"}
			}
			err := client.SendMail("sender@nar.cy", recipients, strings.NewReader("Subject: storage failure\r\n\r\nmessage\r\n"))
			if smtpErr, ok := errors.AsType[*gosmtp.SMTPError](err); !ok || smtpErr.Code != 451 || smtpErr.EnhancedCode != (gosmtp.EnhancedCode{4, 3, 0}) {
				t.Fatalf("expected SMTP 451 4.3.0, got %v", err)
			}
			if strings.Contains(err.Error(), "private backend detail") {
				t.Error("SMTP response exposed backend details")
			}
			for _, mailbox := range []string{"failed@nar.cy", "unattempted@nar.cy"} {
				if _, err := store.List(mailbox); !errors.Is(err, smtpbox.ErrNotFound) {
					t.Fatalf("expected %s absent, got %v", mailbox, err)
				}
			}
			if partial {
				emails, err := store.List("stored@nar.cy")
				if err != nil || len(emails) != 1 {
					t.Fatalf("expected earlier delivery retained, got %d, %v", len(emails), err)
				}
			}
			if err := client.SendMail("sender@nar.cy", []string{"retry@nar.cy"}, strings.NewReader("Subject: after failure\r\n\r\nmessage\r\n")); err != nil {
				t.Fatalf("send after storage failure: %v", err)
			}
			emails, err := store.List("retry@nar.cy")
			if err != nil || len(emails) != 1 || !reflect.DeepEqual(emails[0].To, []string{"retry@nar.cy"}) {
				t.Fatalf("expected clean subsequent delivery, got %v, %v", emails, err)
			}
		})
	}
}

func TestRcptRejectsMissingAddressParts(t *testing.T) {
	s := &session{server: NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())}
	for _, address := range []string{"", "nar.cy", "@nar.cy", "recipient@"} {
		err := s.Rcpt(address, nil)
		if smtpErr, ok := errors.AsType[*gosmtp.SMTPError](err); !ok || smtpErr.Code != 550 {
			t.Errorf("expected recipient %q rejected with 550, got %v", address, err)
		}
	}
	if len(s.rcpt) != 0 {
		t.Errorf("invalid recipients entered envelope: %v", s.rcpt)
	}
}

func TestSMTPRejectsMalformedDomain(t *testing.T) {
	for _, command := range []string{"MAIL", "RCPT"} {
		t.Run(command, func(t *testing.T) {
			store := inmem.NewEmailService()
			client := newDeliveryTestClient(t, "nar.cy", store)
			malformed := "recipient@narity.com@nar.cy"
			var err error
			if command == "MAIL" {
				err = client.Mail(malformed, nil)
			} else {
				if err := client.Mail("sender@nar.cy", nil); err != nil {
					t.Fatal(err)
				}
				err = client.Rcpt(malformed, nil)
			}
			if smtpErr, ok := errors.AsType[*gosmtp.SMTPError](err); !ok || smtpErr.Code != 501 {
				t.Errorf("expected malformed %s domain rejected with 501, got %v", command, err)
			}
			if command == "MAIL" {
				if err := client.Reset(); err != nil {
					t.Fatal(err)
				}
				if err := client.Mail("sender@nar.cy", nil); err != nil {
					t.Fatal(err)
				}
			}
			if err := client.Rcpt("valid@nar.cy", nil); err != nil {
				t.Fatal(err)
			}
			data, err := client.Data()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.Write([]byte("Subject: valid domain\r\n\r\nmessage\r\n")); err != nil {
				t.Fatal(err)
			}
			if err := data.Close(); err != nil {
				t.Fatal(err)
			}
			emails, err := store.List("valid@nar.cy")
			if err != nil || len(emails) != 1 || !reflect.DeepEqual(emails[0].To, []string{"valid@nar.cy"}) {
				t.Errorf("expected only valid recipient stored, got %v, %v", emails, err)
			}
			if _, err := store.List(malformed); !errors.Is(err, smtpbox.ErrNotFound) {
				t.Errorf("expected malformed recipient absent, got %v", err)
			}
		})
	}
}
