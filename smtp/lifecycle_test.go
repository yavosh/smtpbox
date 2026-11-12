package smtp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"github.com/yavosh/smtpbox/inmem"
)

func TestSMTPImmediateStop(t *testing.T) {
	const marker = "smtpbox-immediate-stop"
	if slices.Contains(os.Args, marker) {
		runtime.GOMAXPROCS(1)
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		server := NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())
		t.Cleanup(func() { server.Stop() })
		for range 25 {
			if err := server.StartContext(smtpTestContext(t)); err != nil {
				t.Fatal(err)
			}
			address := server.Addr().String()
			if err := server.Stop(); err != nil {
				t.Fatal(err)
			}
			listener, err := (&net.ListenConfig{}).Listen(smtpTestContext(t), "tcp", address)
			if err != nil {
				t.Fatalf("listener remains bound after shutdown: %v", err)
			}
			listener.Close()
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSMTPImmediateStop$", "--", marker)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("immediate shutdown failed: %v\n%s", err, output)
	}
}

func TestSMTPStartReadyAndStop(t *testing.T) {
	server := NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())
	t.Cleanup(func() { server.Stop() })
	if err := server.StartContext(smtpTestContext(t)); err != nil {
		t.Fatal(err)
	}
	address := server.Addr().String()
	conn, err := (&net.Dialer{}).DialContext(smtpTestContext(t), "tcp", address)
	if err != nil {
		t.Fatalf("listener not ready after Start: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if greeting, err := bufio.NewReader(conn).ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("SMTP greeting unavailable: %q, %v", greeting, err)
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	if n, err := conn.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("active connection remains open after Stop: %d bytes, %v", n, err)
	}
	listener, err := (&net.ListenConfig{}).Listen(smtpTestContext(t), "tcp", address)
	if err != nil {
		t.Fatalf("listener remains bound after Stop: %v", err)
	}
	listener.Close()
	if err := server.Stop(); err == nil {
		t.Error("expected stopping a stopped server to fail")
	}
	if err := server.StartContext(smtpTestContext(t)); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("stop restarted server: %v", err)
	}
}

func TestSMTPCanceledStartup(t *testing.T) {
	server := NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())
	t.Cleanup(func() { server.Stop() })
	if server.Addr() != nil {
		t.Fatal("new server has a listener address")
	}
	ctx, cancel := context.WithCancel(smtpTestContext(t))
	cancel()
	if err := server.StartContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup error = %v, want context.Canceled", err)
	}
	if server.Addr() != nil {
		t.Fatal("canceled startup retained a listener address")
	}
	if err := server.StartContext(smtpTestContext(t)); err != nil {
		t.Fatalf("retry canceled startup: %v", err)
	}
	if server.Addr() == nil {
		t.Fatal("started server has no listener address")
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	if server.Addr() != nil {
		t.Fatal("stopped server retained a listener address")
	}
}

func TestSMTPFailedStartCanRetry(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(smtpTestContext(t), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	server := NewServer(listener.Addr().String(), "nar.cy", inmem.NewEmailService())
	t.Cleanup(func() { server.Stop() })
	if err := server.StartContext(smtpTestContext(t)); err == nil {
		t.Fatal("expected occupied address to fail during Start")
	}
	if server.instance != nil || server.listener != nil || server.serveDone != nil {
		t.Fatal("failed Start retained running state")
	}
	if err := server.Stop(); err == nil {
		t.Error("expected Stop after failed Start to report not running")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.StartContext(smtpTestContext(t)); err != nil {
		t.Fatalf("retry after failed bind: %v", err)
	}
	if err := server.StartContext(smtpTestContext(t)); err == nil {
		t.Error("expected starting a running server to fail")
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestSMTPConcurrentLifecycle(t *testing.T) {
	server := NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())
	t.Cleanup(func() { server.Stop() })
	const workers = 8
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range 25 {
				if err := server.StartContext(smtpTestContext(t)); err != nil && err.Error() != "can't start a running server" {
					errors <- err
					return
				}
				if err := server.Stop(); err != nil && err.Error() != "can't stop, server not running" {
					errors <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if err := server.Stop(); err != nil && err.Error() != "can't stop, server not running" {
		t.Error(err)
	}
	if err := server.StartContext(smtpTestContext(t)); err != nil {
		t.Fatalf("start after concurrent lifecycle calls: %v", err)
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
}

type closeAfterAcceptListener struct {
	net.Listener
	server *gosmtp.Server
}

func (l *closeAfterAcceptListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.server.Close()
	}
	return conn, err
}

func TestSMTPClosesLateAcceptedConnection(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(smtpTestContext(t), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	server := gosmtp.NewServer(&backend{server: NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())})
	t.Cleanup(func() { server.Close() })
	done := make(chan error, 1)
	go func() { done <- server.Serve(&closeAfterAcceptListener{listener, server}) }()
	conn, err := (&net.Dialer{}).DialContext(smtpTestContext(t), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if greeting, err := bufio.NewReader(conn).ReadString('\n'); greeting != "" || !errors.Is(err, io.EOF) {
		t.Errorf("late connection survived shutdown: greeting=%q error=%v", greeting, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SMTP serve: %v", err)
		}
	case <-smtpTestContext(t).Done():
		t.Fatal("serve loop did not exit after shutdown")
	}
}
