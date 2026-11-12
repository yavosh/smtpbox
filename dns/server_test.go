package dns

import (
	"context"
	"net"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

func TestDNSDefaultListenAddress(t *testing.T) {
	server := NewServer(0, "nar.cy")
	if server.server.Addr != "127.0.0.1:0" {
		t.Errorf("DNS default listen address must be loopback, got %q", server.server.Addr)
	}
}

func TestDNSListenHostOptions(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", "::1"} {
		server := NewServer(1053, "nar.cy", WithListenHost(host))
		if server.server.Addr != net.JoinHostPort(host, "1053") {
			t.Errorf("listen host override was not applied for %q", host)
		}
	}
	server := NewServer(0, "nar.cy", WithListenHost(""))
	if err := server.Start(); err == nil {
		server.Stop()
		t.Error("empty DNS listen host must be rejected")
	}
}

func TestDNSDefaultLocalServe(t *testing.T) {
	server := NewServer(0, "nar.cy")
	started := make(chan struct{})
	server.server.NotifyStartedFunc = func() { close(started) }
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("DNS server did not become ready")
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	address := server.server.PacketConn.LocalAddr().(*net.UDPAddr)
	if !address.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("default DNS listener bound outside loopback: %v", address.IP)
	}
	query := new(mdns.Msg)
	query.SetQuestion("nar.cy.", mdns.TypeMX)
	reply, _, err := (&mdns.Client{Timeout: time.Second}).Exchange(query, address.String())
	if err != nil || len(reply.Answer) != 1 {
		t.Errorf("local DNS query failed: %v", err)
	}
}

func startDNSTestServer(t *testing.T, domain string) string {
	t.Helper()
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(0, domain)
	server.server.PacketConn = packet
	started := make(chan struct{})
	server.server.NotifyStartedFunc = func() { close(started) }
	done := make(chan error, 1)
	go func() { done <- server.server.ActivateAndServe() }()
	select {
	case <-started:
	case err := <-done:
		packet.Close()
		t.Fatalf("start DNS server: %v", err)
	case <-time.After(2 * time.Second):
		packet.Close()
		t.Fatal("DNS server did not become ready")
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return packet.LocalAddr().String()
}

func TestDNSStandardResolver(t *testing.T) {
	address := startDNSTestServer(t, "nar.cy")
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mxs, err := resolver.LookupMX(ctx, "nar.cy.")
	if err != nil {
		t.Errorf("standard MX lookup: %v", err)
	} else if len(mxs) != 1 || mxs[0].Host != "mail.nar.cy." || mxs[0].Pref != 10 {
		t.Errorf("unexpected MX response: %v", mxs)
	}
	addresses, err := resolver.LookupIP(ctx, "ip4", "mail.nar.cy.")
	if err != nil {
		t.Errorf("standard MX host lookup: %v", err)
	} else if len(addresses) != 1 || !addresses[0].Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("unexpected MX host addresses: %v", addresses)
	}
}

func TestDNSConfiguredNames(t *testing.T) {
	for _, configured := range []string{"nar.cy", "NaR.Cy.", "nar.cy."} {
		t.Run(configured, func(t *testing.T) {
			address := startDNSTestServer(t, configured)
			for _, tc := range []struct {
				name  string
				qtype uint16
			}{
				{"nar.cy.", mdns.TypeMX},
				{"NAR.CY.", mdns.TypeMX},
				{"NaR.Cy.", mdns.TypeA},
				{"MaIl.NaR.Cy.", mdns.TypeA},
			} {
				query := new(mdns.Msg)
				query.SetQuestion(tc.name, tc.qtype)
				reply, _, err := (&mdns.Client{Timeout: time.Second}).Exchange(query, address)
				if err != nil {
					t.Fatalf("query %s type %d: %v", tc.name, tc.qtype, err)
				}
				if reply.Rcode != mdns.RcodeSuccess || !reply.Authoritative || len(reply.Answer) != 1 || len(reply.Extra) != 0 {
					t.Fatalf("expected one authoritative answer, got %s", reply)
				}
				header := reply.Answer[0].Header()
				if header.Name != tc.name || header.Rrtype != tc.qtype || header.Class != mdns.ClassINET || header.Ttl != 60 {
					t.Errorf("incorrect answer header: %+v", header)
				}
				switch record := reply.Answer[0].(type) {
				case *mdns.MX:
					if record.Mx != "mail.nar.cy." || record.Preference != 10 {
						t.Errorf("incorrect MX record: %+v", record)
					}
				case *mdns.A:
					if !record.A.Equal(net.IPv4(127, 0, 0, 1)) {
						t.Errorf("incorrect A address: %v", record.A)
					}
				default:
					t.Errorf("unexpected answer type: %T", record)
				}
			}
		})
	}
}

func TestDNSNegativeResponses(t *testing.T) {
	address := startDNSTestServer(t, "nar.cy")
	for _, tc := range []struct {
		name          string
		qtype         uint16
		qclass        uint16
		rcode         int
		authoritative bool
	}{
		{"nar.cy.", mdns.TypeAAAA, mdns.ClassINET, mdns.RcodeSuccess, true},
		{"mail.nar.cy.", mdns.TypeMX, mdns.ClassINET, mdns.RcodeSuccess, true},
		{"mail.nar.cy.", mdns.TypeTXT, mdns.ClassINET, mdns.RcodeSuccess, true},
		{"missing.nar.cy.", mdns.TypeA, mdns.ClassINET, mdns.RcodeNameError, true},
		{"missing.nar.cy.", mdns.TypeMX, mdns.ClassINET, mdns.RcodeNameError, true},
		{"example.com.", mdns.TypeA, mdns.ClassINET, mdns.RcodeRefused, false},
		{"example.org.", mdns.TypeA, mdns.ClassINET, mdns.RcodeRefused, false},
		{"example.net.", mdns.TypeA, mdns.ClassINET, mdns.RcodeRefused, false},
		{"mail.example.net.", mdns.TypeA, mdns.ClassINET, mdns.RcodeRefused, false},
		{"narity.com.", mdns.TypeMX, mdns.ClassINET, mdns.RcodeRefused, false},
		{"evilnar.cy.", mdns.TypeA, mdns.ClassINET, mdns.RcodeRefused, false},
		{"nar.cy.", mdns.TypeA, mdns.ClassCHAOS, mdns.RcodeRefused, false},
	} {
		t.Run(tc.name+"/"+mdns.TypeToString[tc.qtype], func(t *testing.T) {
			query := new(mdns.Msg)
			query.SetQuestion(tc.name, tc.qtype)
			query.Question[0].Qclass = tc.qclass
			reply, _, err := (&mdns.Client{Timeout: time.Second}).Exchange(query, address)
			if err != nil {
				t.Fatal(err)
			}
			if reply.Rcode != tc.rcode || reply.Authoritative != tc.authoritative || len(reply.Answer) != 0 || len(reply.Extra) != 0 {
				t.Errorf("incorrect negative response: %s", reply)
			}
		})
	}
}
