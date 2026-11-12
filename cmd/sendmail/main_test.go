package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox"
	"github.com/yavosh/smtpbox/inmem"
	"github.com/yavosh/smtpbox/smtp"
)

func startSendmailTestServer(t *testing.T) (string, smtpbox.EmailStoreBackend) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	store := inmem.NewEmailService()
	server := smtp.NewServer(address, "nar.cy", store)
	server.Start()
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return address, store
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for SMTP server: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSendmailCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "sendmail")
	if output, err := exec.Command("go", "build", "-mod=vendor", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build sendmail: %v\n%s", err, output)
	}
	address, store := startSendmailTestServer(t)
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	message := "Subject: local sender\r\n\r\nmessage body\r\n"
	for _, tc := range []struct {
		name       string
		recipients string
		auth       bool
		wantError  bool
	}{
		{"plain", "plain@nar.cy", false, false},
		{"test-auth", "authenticated@nar.cy", true, false},
		{"multiple-recipients", "first@nar.cy,second@nar.cy", false, false},
		{"rejected-recipient", "rejected@narity.com", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"-smtp-host", host, "-smtp-port", port, "-email-from", "sender@nar.cy", "-email-recipient", tc.recipients, "-email-body", message}
			if tc.auth {
				args = append(args, "-sasl-username", "test-user", "-sasl-password", "test-only")
			}
			output, err := exec.Command(binary, args...).CombinedOutput()
			if tc.wantError {
				if err == nil || !strings.Contains(string(output), "550") {
					t.Fatalf("expected recipient rejection, got %v\n%s", err, output)
				}
				if strings.Contains(string(output), "Email sent to") {
					t.Error("failed delivery reported success")
				}
				return
			}
			if err != nil {
				t.Fatalf("sendmail failed: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), "Email sent to "+tc.recipients) {
				t.Errorf("missing delivery success output: %s", output)
			}
			for _, mailbox := range strings.Split(tc.recipients, ",") {
				emails, err := store.List(mailbox)
				if err != nil || len(emails) != 1 {
					t.Fatalf("expected one message in %s, got %d, %v", mailbox, len(emails), err)
				}
				if emails[0].Body != message || emails[0].From != "sender@nar.cy" {
					t.Error("delivered message differs from command input")
				}
			}
		})
	}
	t.Run("default-recipient", func(t *testing.T) {
		output, err := exec.Command(binary, "-help").CombinedOutput()
		if err != nil {
			t.Fatalf("sendmail help: %v", err)
		}
		if !strings.Contains(string(output), `(default "user@localhost")`) {
			t.Error("default recipient does not match the default server domain")
		}
	})
}

type errorMessageReader struct {
	err error
}

func (r errorMessageReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestSendMailClosesOnFailure(t *testing.T) {
	for _, phase := range []string{"auth", "recipient", "body", "quit"} {
		t.Run(phase, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				reader := bufio.NewReader(conn)
				if _, err := io.WriteString(conn, "220 local test SMTP\r\n"); err != nil {
					done <- err
					return
				}
				type smtpStep struct {
					prefix   string
					response string
				}
				steps := []smtpStep{
					{"EHLO ", "250-local test SMTP\r\n250 AUTH PLAIN\r\n"},
				}
				if phase == "auth" {
					steps = append(steps,
						smtpStep{"AUTH PLAIN", "535 5.7.8 Test authentication failure\r\n"},
						smtpStep{"*", "501 Authentication aborted\r\n"},
					)
				} else {
					steps = append(steps, smtpStep{"MAIL FROM:", "250 Sender accepted\r\n"})
					response := "250 Recipient accepted\r\n"
					if phase == "recipient" {
						response = "550 5.1.2 Test recipient failure\r\n"
					}
					steps = append(steps, smtpStep{"RCPT TO:", response})
					if phase != "recipient" {
						steps = append(steps, smtpStep{"DATA", "354 Send message\r\n"})
					}
				}
				for _, step := range steps {
					line, err := reader.ReadString('\n')
					if err != nil {
						done <- err
						return
					}
					if !strings.HasPrefix(line, step.prefix) {
						done <- fmt.Errorf("expected %s command", step.prefix)
						return
					}
					if _, err := io.WriteString(conn, step.response); err != nil {
						done <- err
						return
					}
				}
				if phase == "quit" {
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							done <- err
							return
						}
						if line == ".\r\n" {
							break
						}
					}
					if _, err := io.WriteString(conn, "250 Message accepted\r\n"); err != nil {
						done <- err
						return
					}
					line, err := reader.ReadString('\n')
					if err != nil {
						done <- err
						return
					}
					if line != "QUIT\r\n" {
						done <- errors.New("expected QUIT command")
						return
					}
					if _, err := io.WriteString(conn, "500 Test quit failure\r\n"); err != nil {
						done <- err
						return
					}
				}
				if _, err := reader.ReadString('\n'); !errors.Is(err, io.EOF) {
					done <- fmt.Errorf("client connection remained open: %v", err)
					return
				}
				done <- nil
			}()

			var auth sasl.Client
			if phase == "auth" {
				auth = sasl.NewPlainClient("", "test-user", "test-only")
			}
			messageErr := errors.New("test message read failure")
			var message io.Reader = strings.NewReader("Subject: cleanup\r\n\r\nmessage\r\n")
			if phase == "body" {
				message = errorMessageReader{messageErr}
			}
			err = sendMail(listener.Addr().String(), auth, "sender@nar.cy", []string{"recipient@nar.cy"}, message)
			if phase == "body" {
				if !errors.Is(err, messageErr) {
					t.Errorf("expected message read failure, got %v", err)
				}
			} else {
				wantCode := map[string]int{"auth": 535, "recipient": 550, "quit": 500}[phase]
				var smtpErr *gosmtp.SMTPError
				if !errors.As(err, &smtpErr) || smtpErr.Code != wantCode {
					t.Errorf("expected SMTP %d, got %v", wantCode, err)
				}
			}
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
}
