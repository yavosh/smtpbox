package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
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
	store := inmem.NewEmailService()
	server := smtp.NewServer("127.0.0.1:0", "nar.cy", store)
	if err := server.StartContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	return server.Addr().String(), store
}

func TestSendmailCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "sendmail")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	if output, err := exec.CommandContext(ctx, "go", "build", "-mod=vendor", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build sendmail: %v\n%s", err, output)
	}
	address, store := startSendmailTestServer(t)
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	defaultMessage := "Subject: local sender\r\n\r\nmessage body\r\n"
	for _, tc := range []struct {
		name       string
		recipients string
		subject    string
		message    string
		auth       bool
		wantError  bool
	}{
		{name: "plain", recipients: "plain@nar.cy"},
		{name: "raw-body", recipients: "raw@nar.cy", message: "raw body\r\n"},
		{name: "subject", recipients: "subject@nar.cy", subject: "replacement subject"},
		{name: "test-auth", recipients: "authenticated@nar.cy", auth: true},
		{name: "multiple-recipients", recipients: "first@nar.cy,second@nar.cy"},
		{name: "rejected-recipient", recipients: "rejected@narity.com", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			message := tc.message
			if message == "" {
				message = defaultMessage
			}
			args := []string{"-smtp-host", host, "-smtp-port", port, "-email-from", "sender@nar.cy", "-email-recipient", tc.recipients, "-email-body", message}
			if tc.subject != "" {
				args = append(args, "-email-subject", tc.subject)
			}
			if tc.auth {
				args = append(args, "-sasl-username", "test-user", "-sasl-password", "test-only")
			}
			output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
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
			for mailbox := range strings.SplitSeq(tc.recipients, ",") {
				emails, err := store.List(mailbox)
				if err != nil || len(emails) != 1 {
					t.Fatalf("expected one message in %s, got %d, %v", mailbox, len(emails), err)
				}
				wantMessage := message
				if tc.subject != "" {
					wantMessage = "Subject: " + tc.subject + "\r\n\r\nmessage body\r\n"
				}
				if emails[0].Body != wantMessage || emails[0].From != "sender@nar.cy" {
					t.Error("delivered message differs from command input")
				}
			}
		})
	}
	t.Run("default-recipient", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		t.Cleanup(cancel)
		output, err := exec.CommandContext(ctx, binary, "-help").CombinedOutput()
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
			listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
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
					done <- fmt.Errorf("client connection remained open: %w", err)
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
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			err = sendMail(ctx, listener.Addr().String(), auth, "sender@nar.cy", []string{"recipient@nar.cy"}, message)
			if phase == "body" {
				if !errors.Is(err, messageErr) {
					t.Errorf("expected message read failure, got %v", err)
				}
			} else {
				wantCode := map[string]int{"auth": 535, "recipient": 550, "quit": 500}[phase]
				if smtpErr, ok := errors.AsType[*gosmtp.SMTPError](err); !ok || smtpErr.Code != wantCode {
					t.Errorf("expected SMTP %d, got %v", wantCode, err)
				}
			}
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
}

func TestWithSubject(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		subject string
		want    string
	}{
		{"raw", "raw body\nwithout headers", "", "raw body\nwithout headers"},
		{"plain", "raw body", "new", "Subject: new\r\n\r\nraw body"},
		{"plain-paragraphs", "first paragraph\n\nsecond paragraph", "new", "Subject: new\r\n\r\nfirst paragraph\n\nsecond paragraph"},
		{"headers", "From: sender@nar.cy\r\nX-Test: unchanged\r\n\r\nbody", "new", "Subject: new\r\nFrom: sender@nar.cy\r\nX-Test: unchanged\r\n\r\nbody"},
		{"folded", "From: sender@nar.cy\r\nsUbJeCt: old\r\n\tcontinued\r\nX-Test: unchanged\r\n\r\nbody", "new", "From: sender@nar.cy\r\nSubject: new\r\nX-Test: unchanged\r\n\r\nbody"},
		{"last-header", "From: sender@nar.cy\r\nSubject: old\r\n\r\nbody", "new", "From: sender@nar.cy\r\nSubject: new\r\n\r\nbody"},
		{"lf", "Subject: old\nX-Test: unchanged\n\nbody\r\n\r\nparagraph", "new", "Subject: new\nX-Test: unchanged\n\nbody\r\n\r\nparagraph"},
		{"mixed", "Subject: old\nX-Test: keep\r\n\r\nbody", "new", "Subject: new\r\nX-Test: keep\r\n\r\nbody"},
		{"mixed-reversed", "From: x@nar.cy\nSubject: old\r\n\r\nbody", "new", "From: x@nar.cy\nSubject: new\r\n\r\nbody"},
		{"mixed-folded", "X-Test: keep\r\nSubject: old\n\tcontinued\r\nX-Other: unchanged\n\nbody\r\n", "new", "X-Test: keep\r\nSubject: new\nX-Other: unchanged\n\nbody\r\n"},
		{"mixed-folded-crlf", "From: x@nar.cy\nSubject: old\r\n\tcontinued\nX-Test: keep\r\n\r\nbody\n\r\n", "new", "From: x@nar.cy\nSubject: new\r\nX-Test: keep\r\n\r\nbody\n\r\n"},
		{"duplicate", "Subject: old\r\nSubject: duplicate\r\n\r\nbody", "new", "Subject: new\r\n\r\nbody"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := withSubject(tc.message, tc.subject)
			if err != nil || got != tc.want {
				t.Fatalf("withSubject() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, subject := range []string{"injected\rheader", "injected\nheader"} {
		if _, err := withSubject("body", subject); err == nil {
			t.Errorf("withSubject accepted CR/LF: %q", subject)
		}
	}
	t.Run("unicode", func(t *testing.T) {
		got, err := withSubject("body", "Καλημέρα")
		if err != nil {
			t.Fatal(err)
		}
		message, err := mail.ReadMessage(strings.NewReader(got))
		if err != nil {
			t.Fatal(err)
		}
		subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
		if err != nil || subject != "Καλημέρα" {
			t.Fatalf("decoded subject = %q, %v", subject, err)
		}
	})
}

func TestRunHelp(t *testing.T) {
	for range 2 {
		var stdout, stderr bytes.Buffer
		if err := run(t.Context(), []string{"-help"}, &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), `(default "user@localhost")`) {
			t.Fatalf("unexpected help output: stdout %q, stderr %q", stdout.String(), stderr.String())
		}
	}
}

func TestRunRejectsSubjectInjection(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-email-subject", "unsafe\r\nX-Injected: true"}, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("run() = %v, stdout %q", err, stdout.String())
	}
}

func TestRunIPv6(t *testing.T) {
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	accepted := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			cancel()
			conn.Close()
		}
		accepted <- err
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err = run(ctx, []string{"-smtp-host", "::1", "-smtp-port", port}, &stdout, &stderr)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() = %v, want cancellation after IPv6 connection", err)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
}

func TestSendMailCancellation(t *testing.T) {
	for _, phase := range []string{"greeting", "command", "body", "submission", "deadline"} {
		t.Run(phase, func(t *testing.T) {
			listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			if phase == "deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 250*time.Millisecond)
			}
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				if phase != "greeting" && phase != "deadline" {
					if _, err := io.WriteString(conn, "220 local test SMTP\r\n"); err != nil {
						done <- err
						return
					}
					inData := false
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							done <- err
							return
						}
						if phase == "command" || line == ".\r\n" {
							break
						}
						if inData {
							continue
						}
						response := "250 OK\r\n"
						if line == "DATA\r\n" {
							response = "354 Send message\r\n"
							inData = true
						}
						if _, err := io.WriteString(conn, response); err != nil {
							done <- err
							return
						}
						if phase == "body" && line == "DATA\r\n" {
							if _, err := reader.ReadByte(); err != nil {
								done <- err
								return
							}
							break
						}
					}
				}
				if phase != "deadline" {
					cancel()
				}
				_, err = io.Copy(io.Discard, reader)
				if err != nil {
					done <- fmt.Errorf("connection after cancellation: %w", err)
					return
				}
				done <- nil
			}()
			message := "body\r\n"
			if phase == "body" {
				message = strings.Repeat("x", 16<<20)
			}
			err = sendMail(ctx, listener.Addr().String(), nil, "sender@nar.cy", []string{"recipient@nar.cy"}, strings.NewReader(message))
			wantError := context.Canceled
			if phase == "deadline" {
				wantError = context.DeadlineExceeded
			}
			if !errors.Is(err, wantError) {
				t.Errorf("sendMail() = %v, want %v", err, wantError)
			}
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
}
