package smtp

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestSMTPClientFixtureCancellation(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(smtpTestContext(t), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	ctx, cancel := context.WithCancel(smtpTestContext(t))
	t.Cleanup(cancel)
	client := newSMTPTestClient(t, ctx, listener.Addr().String())
	peer, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peer.Close() })

	// The peer accepts the connection and withholds its greeting.
	// The client must stop when the fixture context is canceled.
	started := make(chan struct{})
	exited := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		defer close(exited)
		close(started)
		result <- client.Hello("localhost")
	}()
	t.Cleanup(func() {
		peer.Close()
		<-exited
	})
	<-started
	cancel()
	watchdog, stop := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(stop)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stalled greeting unexpectedly succeeded")
		}
	case <-watchdog.Done():
		t.Fatal("client remained blocked after fixture cancellation")
	}
}
