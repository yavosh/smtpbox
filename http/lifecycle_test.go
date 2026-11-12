package http

import (
	"context"
	"errors"
	"io"
	"net"
	stdhttp "net/http"
	"testing"
	"time"

	"github.com/yavosh/smtpbox/inmem"
)

func TestHTTPStartContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	server := NewServer(0, inmem.NewEmailService())
	if err := server.StartContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("StartContext() = %v, want context.Canceled", err)
	}
	if addr := server.Addr(); addr != nil {
		t.Errorf("Addr() = %v after canceled start, want nil", addr)
	}
}

func TestHTTPStartContextReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	server := NewServer(0, inmem.NewEmailService())
	if err := server.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	addr := server.Addr()
	if addr == nil {
		t.Fatal("Addr() is nil after starting")
	}
	request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, "http://"+addr.String()+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &stdhttp.Client{Timeout: time.Second}
	t.Cleanup(client.CloseIdleConnections)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != stdhttp.StatusOK || string(body) != `{"result":{"status":"ok"}}` {
		t.Errorf("GET /health = %d %s, want 200 with healthy JSON", response.StatusCode, body)
	}
}

func TestHTTPImmediateStopClosesListener(t *testing.T) {
	for range 20 {
		server := NewServer(0, inmem.NewEmailService())
		if err := server.StartContext(t.Context()); err != nil {
			t.Fatal(err)
		}
		addr := server.Addr()
		if err := server.Stop(); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr.String())
		cancel()
		if err == nil {
			conn.Close()
			t.Fatalf("listener %s accepted a connection after Stop", addr)
		}
	}
}

func TestHTTPStopClosesActiveConnectionsAfterTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	entered := make(chan struct{})
	stopped := make(chan struct{})
	server := NewServer(0, inmem.NewEmailService())
	server.server.Handler = stdhttp.HandlerFunc(func(_ stdhttp.ResponseWriter, request *stdhttp.Request) {
		close(entered)
		<-request.Context().Done()
		close(stopped)
	})
	if err := server.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		server.server.Close()
		server.Stop()
	})
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", server.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request did not reach the handler")
	}
	if err := server.Stop(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() = %v, want context.DeadlineExceeded", err)
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("Stop left the active request open after its deadline")
	}
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("connection read after Stop = %v, want EOF", err)
	}
}
