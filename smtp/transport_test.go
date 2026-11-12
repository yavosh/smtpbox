package smtp

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox"
	"github.com/yavosh/smtpbox/inmem"
)

func TestSMTPMessageSize(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	store := inmem.NewEmailService()
	server := NewServer(address, "nar.cy", store)
	server.Start()
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})

	var client *gosmtp.Client
	deadline := time.Now().Add(2 * time.Second)
	for {
		client, err = gosmtp.Dial(address)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial SMTP server: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	t.Cleanup(func() { client.Close() })

	const maxBytes = 1024 * 1024
	if ok, size := client.Extension("SIZE"); !ok || size != strconv.Itoa(maxBytes) {
		t.Fatalf("expected SIZE %d, got advertised=%v size=%q", maxBytes, ok, size)
	}

	for _, tc := range []struct {
		name       string
		size       int
		dotStuffed bool
	}{
		{"above-old-cap", 20481, false},
		{"large", 64 * 1024, false},
		{"below-limit", maxBytes - 1, false},
		{"at-limit", maxBytes, false},
		{"above-limit", maxBytes + 1, false},
		{"at-limit-dot-stuffed", maxBytes, true},
		{"above-limit-dot-stuffed", maxBytes + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := "Subject: message size\r\n\r\n"
			remaining := tc.size - len(header) - 2
			line := "0123456789\r\n"
			if tc.dotStuffed {
				line = ".123456789\r\n"
			}
			message := header + strings.Repeat(line, remaining/len(line)) + strings.Repeat("a", remaining%len(line)) + "\r\n"
			recipient := tc.name + "@nar.cy"
			err := client.SendMail("sender@nar.cy", []string{recipient}, strings.NewReader(message))
			if tc.size > maxBytes {
				var smtpErr *gosmtp.SMTPError
				if !errors.As(err, &smtpErr) || smtpErr.Code != 552 {
					t.Fatalf("expected SMTP 552, got %v", err)
				}
				if _, err := store.List(recipient); !errors.Is(err, smtpbox.ErrNotFound) {
					t.Fatalf("expected no stored message, got %v", err)
				}
				message = "Subject: after rejection\r\n\r\ncomplete\r\n"
				if err := client.SendMail("sender@nar.cy", []string{recipient}, strings.NewReader(message)); err != nil {
					t.Fatalf("send after oversized message: %v", err)
				}
			} else if err != nil {
				t.Fatalf("send message: %v", err)
			}

			emails, err := store.List(recipient)
			if err != nil {
				t.Fatal(err)
			}
			if len(emails) != 1 {
				t.Fatalf("expected one message, got %d", len(emails))
			}
			if emails[0].Body != message {
				t.Errorf("stored message differs: expected %d bytes, got %d", len(message), len(emails[0].Body))
			}
		})
	}
}
