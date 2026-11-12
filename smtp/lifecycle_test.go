package smtp

import (
	"bufio"
	"context"
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
		for range 25 {
			if err := server.Start(); err != nil {
				t.Fatal(err)
			}
			address := server.listener.Addr().String()
			if err := server.Stop(); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("listener remains bound after shutdown: %v", err)
			}
			listener.Close()
			time.Sleep(time.Millisecond)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSMTPImmediateStop$", "--", marker)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("immediate shutdown failed: %v\n%s", err, output)
	}
}

func TestSMTPStartReadyAndStop(t *testing.T) {
	server := NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())
	t.Cleanup(func() { server.Stop() })
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	address := server.listener.Addr().String()
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("listener not ready after Start: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if greeting, err := bufio.NewReader(conn).ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("SMTP greeting unavailable: %q, %v", greeting, err)
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	if n, err := conn.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Errorf("active connection remains open after Stop: %d bytes, %v", n, err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listener remains bound after Stop: %v", err)
	}
	listener.Close()
	if err := server.Stop(); err == nil {
		t.Error("expected stopping a stopped server to fail")
	}
	if err := server.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("stop restarted server: %v", err)
	}
}

func TestSMTPFailedStartCanRetry(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := NewServer(listener.Addr().String(), "nar.cy", inmem.NewEmailService())
	t.Cleanup(func() { server.Stop() })
	if err := server.Start(); err == nil {
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
	if err := server.Start(); err != nil {
		t.Fatalf("retry after failed bind: %v", err)
	}
	if err := server.Start(); err == nil {
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				if err := server.Start(); err != nil && err.Error() != "can't start a running server" {
					errors <- err
					return
				}
				if err := server.Stop(); err != nil && err.Error() != "can't stop, server not running" {
					errors <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if err := server.Stop(); err != nil && err.Error() != "can't stop, server not running" {
		t.Error(err)
	}
	if err := server.Start(); err != nil {
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := gosmtp.NewServer(&backend{server: NewServer("127.0.0.1:0", "nar.cy", inmem.NewEmailService())})
	done := make(chan error, 1)
	go func() { done <- server.Serve(&closeAfterAcceptListener{listener, server}) }()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if greeting, err := bufio.NewReader(conn).ReadString('\n'); greeting != "" || err != io.EOF {
		t.Errorf("late connection survived shutdown: greeting=%q error=%v", greeting, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SMTP serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve loop did not exit after shutdown")
	}
}
