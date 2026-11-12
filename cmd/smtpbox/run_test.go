package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"
)

type readinessHandler struct {
	ready chan map[string]string
}

func (h readinessHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h readinessHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == "smtpbox ready" {
		addresses := make(map[string]string)
		record.Attrs(func(attr slog.Attr) bool {
			addresses[attr.Key] = attr.Value.String()
			return true
		})
		h.ready <- addresses
	}
	return nil
}

func (h readinessHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h readinessHandler) WithGroup(string) slog.Handler      { return h }

func startTestApp(t *testing.T, flags []string, environment map[string]string) (map[string]string, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	ready := make(chan map[string]string, 1)
	done := make(chan struct{})
	var runErr error
	args := []string{"-http-port", "0", "-smtp-port", "0", "-dns-port", "0", "-smtp-domain", "nar.cy", "-dns-domain", "nar.cy"}
	lookupEnv := func(key string) (string, bool) {
		value, present := environment[key]
		return value, present
	}
	go func() {
		runErr = run(ctx, append(args, flags...), lookupEnv, io.Discard, slog.New(readinessHandler{ready: ready}))
		close(done)
	}()
	stop := func() {
		t.Helper()
		cancel()
		select {
		case <-done:
			if runErr != nil {
				t.Errorf("application shutdown: %v", runErr)
			}
		case <-time.After(10 * time.Second):
			t.Error("application did not stop after cancellation")
		}
	}
	t.Cleanup(stop)
	select {
	case addresses := <-ready:
		return addresses, stop
	case <-done:
		t.Fatalf("application stopped before readiness: %v", runErr)
	case <-ctx.Done():
		t.Fatalf("wait for application readiness: %v", ctx.Err())
	}
	return nil, stop
}

func startNetworkApp(t *testing.T, flags []string, environment map[string]string) (string, *gosmtp.Client) {
	t.Helper()
	addresses, _ := startTestApp(t, flags, environment)
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", addresses["smtp_addr"])
	if err != nil {
		t.Fatal(err)
	}
	client := gosmtp.NewClient(conn)
	client.CommandTimeout = time.Second
	client.SubmissionTimeout = time.Second
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Noop(); err != nil {
		t.Fatalf("SMTP not ready after application readiness: %v", err)
	}
	return "http://" + addresses["http_addr"], client
}

func TestRunCancellationReleasesListeners(t *testing.T) {
	addresses, stop := startTestApp(t, nil, nil)
	stop()
	for _, service := range []string{"http_addr", "smtp_addr"} {
		listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", addresses[service])
		if err != nil {
			t.Fatalf("%s still bound after cancellation: %v", service, err)
		}
		_ = listener.Close()
	}
	packet, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", addresses["dns_addr"])
	if err != nil {
		t.Fatalf("DNS still bound after cancellation: %v", err)
	}
	_ = packet.Close()
}

func TestRunReportsDNSBindFailure(t *testing.T) {
	packet, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = packet.Close() })
	port := packet.LocalAddr().(*net.UDPAddr).Port
	args := []string{"-http-port", "0", "-smtp-port", "0", "-dns-port", strconv.Itoa(port)}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	err = run(ctx, args, func(string) (string, bool) { return "", false }, io.Discard, slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "start dns") {
		t.Fatalf("occupied DNS port: got %v, want DNS startup failure", err)
	}
}

func TestRunCanceledBeforeStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := run(ctx, nil, func(string) (string, bool) { return "", false }, io.Discard, slog.New(slog.DiscardHandler))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup: got %v, want context.Canceled", err)
	}
}

func TestConfigFlagsAreIndependent(t *testing.T) {
	lookupEnv := func(string) (string, bool) { return "", false }
	first, err := parseConfig([]string{"-http-port", "1234"}, lookupEnv, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parseConfig(nil, lookupEnv, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if first.httpPort != 1234 || second.httpPort != 8080 {
		t.Errorf("private flag sets: got ports %d and %d, want 1234 and 8080", first.httpPort, second.httpPort)
	}
}
