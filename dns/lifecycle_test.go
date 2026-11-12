package dns

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	mdns "github.com/miekg/dns"
)

func TestDNSStartOccupiedPort(t *testing.T) {
	packet, err := (&net.ListenConfig{}).ListenPacket(dnsTestContext(t), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = packet.Close() })
	server := NewServer(packet.LocalAddr().(*net.UDPAddr).Port, "nar.cy")
	t.Cleanup(func() { _ = server.Stop() })
	if err := server.Start(); err == nil {
		t.Fatal("Start succeeded with an occupied UDP port")
	}
	if server.Addr() != nil {
		t.Fatal("failed startup retained a listener address")
	}
	assertDNSNotStarted(t, server.Stop())
	if err := packet.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.StartContext(dnsTestContext(t)); err != nil {
		t.Fatalf("retry after releasing port: %v", err)
	}
	queryDNSLifecycleServer(t, server)
}

func queryDNSLifecycleServer(t *testing.T, server *Server) {
	t.Helper()
	query := new(mdns.Msg)
	query.SetQuestion("nar.cy.", mdns.TypeMX)
	address := server.Addr()
	if address == nil {
		t.Fatal("running server has no address")
	}
	reply, _, err := (&mdns.Client{}).ExchangeContext(dnsTestContext(t), query, address.String())
	if err != nil {
		t.Fatalf("query ready server: %v", err)
	}
	if len(reply.Answer) != 1 {
		t.Fatalf("ready server returned %d answers, want 1", len(reply.Answer))
	}
}

func TestDNSLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query bool
	}{
		{name: "immediate stop"},
		{name: "ready query", query: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(0, "nar.cy")
			t.Cleanup(func() { _ = server.Stop() })
			for range 3 {
				if err := server.StartContext(dnsTestContext(t)); err != nil {
					t.Fatal(err)
				}
				address := server.Addr()
				if address == nil {
					t.Fatal("started server has no address")
				}
				if tc.query {
					queryDNSLifecycleServer(t, server)
				}
				if err := server.Stop(); err != nil {
					t.Fatal(err)
				}
				if server.Addr() != nil {
					t.Fatal("stopped server retained a listener address")
				}
				packet, err := (&net.ListenConfig{}).ListenPacket(dnsTestContext(t), "udp", address.String())
				if err != nil {
					t.Fatalf("rebind stopped listener: %v", err)
				}
				if err := packet.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestDNSCanceledStartup(t *testing.T) {
	server := NewServer(0, "nar.cy")
	t.Cleanup(func() { _ = server.Stop() })
	ctx, cancel := context.WithCancel(dnsTestContext(t))
	cancel()
	if err := server.StartContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartContext error = %v, want context.Canceled", err)
	}
	if server.Addr() != nil {
		t.Fatal("canceled startup retained a listener address")
	}
	assertDNSNotStarted(t, server.Stop())
	if err := server.StartContext(dnsTestContext(t)); err != nil {
		t.Fatalf("retry canceled startup: %v", err)
	}
	queryDNSLifecycleServer(t, server)
}

func assertDNSNotStarted(t *testing.T, err error) {
	t.Helper()
	if _, ok := errors.AsType[*mdns.Error](err); !ok || err.Error() != "dns: server not started" {
		t.Errorf("Stop error = %v, want dns: server not started", err)
	}
}

func TestDNSStopNotStarted(t *testing.T) {
	for _, started := range []bool{false, true} {
		name := "before startup"
		if started {
			name = "after shutdown"
		}
		t.Run(name, func(t *testing.T) {
			server := NewServer(0, "nar.cy")
			t.Cleanup(func() { _ = server.Stop() })
			if started {
				if err := server.StartContext(dnsTestContext(t)); err != nil {
					t.Fatal(err)
				}
				if err := server.Stop(); err != nil {
					t.Fatal(err)
				}
			}
			assertDNSNotStarted(t, server.Stop())
		})
	}
}

func TestDNSStartupContextLifetime(t *testing.T) {
	server := NewServer(0, "nar.cy")
	t.Cleanup(func() { _ = server.Stop() })
	ctx, cancel := context.WithCancel(dnsTestContext(t))
	if err := server.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	queryDNSLifecycleServer(t, server)
}

func TestDNSInvalidListenAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		port int
		host string
	}{
		{name: "invalid host", host: "127.0.0.1:invalid"},
		{name: "invalid port", port: -1, host: "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(tc.port, "nar.cy", WithListenHost(tc.host))
			t.Cleanup(func() { _ = server.Stop() })
			if err := server.StartContext(dnsTestContext(t)); err == nil {
				t.Fatal("StartContext accepted an invalid listen address")
			}
			if server.Addr() != nil {
				t.Fatal("invalid startup retained a listener address")
			}
		})
	}
}

func TestDNSConcurrentLifecycle(t *testing.T) {
	server := NewServer(0, "nar.cy")
	t.Cleanup(func() { _ = server.Stop() })
	ctx := dnsTestContext(t)
	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			<-start
			results <- server.StartContext(ctx)
		})
	}
	close(start)
	wg.Wait()
	close(results)
	started := 0
	for err := range results {
		if err == nil {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("concurrent starts succeeded %d times, want 1", started)
	}
	queryDNSLifecycleServer(t, server)
	stopResults := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			_ = server.Addr()
			stopResults <- server.Stop()
		})
	}
	wg.Wait()
	close(stopResults)
	stopped := 0
	for err := range stopResults {
		if err == nil {
			stopped++
		} else {
			assertDNSNotStarted(t, err)
		}
	}
	if stopped != 1 {
		t.Fatalf("concurrent stops succeeded %d times, want 1", stopped)
	}
	if err := server.StartContext(ctx); err != nil {
		t.Fatalf("restart after concurrent stops: %v", err)
	}
	queryDNSLifecycleServer(t, server)
}
