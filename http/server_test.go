package http

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yavosh/smtpbox"
	"github.com/yavosh/smtpbox/inmem"
)

func TestHTTPDefaults(t *testing.T) {
	server := NewServer(0, inmem.NewEmailService())
	if server.server.Addr != "127.0.0.1:0" {
		t.Errorf("HTTP default listen address must be loopback, got %q", server.server.Addr)
	}
	if server.server.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("expected 5-second header timeout, got %v", server.server.ReadHeaderTimeout)
	}
	if server.server.DisableGeneralOptionsHandler {
		t.Error("default unauthenticated OPTIONS handling must remain enabled")
	}
}

func TestHTTPListenHostOptions(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", "::1"} {
		server := NewServer(8080, inmem.NewEmailService(), WithListenHost(host))
		if server.server.Addr != net.JoinHostPort(host, "8080") {
			t.Errorf("listen host override was not applied for %q", host)
		}
	}
	for _, host := range []string{"", "bad:host"} {
		server := NewServer(0, inmem.NewEmailService(), WithListenHost(host))
		if err := server.Start(); err == nil {
			server.Stop()
			t.Errorf("invalid listen host was accepted: %q", host)
		}
	}
}

func TestHTTPAuthenticationProtectsAllRoutes(t *testing.T) {
	const username = "fixture-reader"
	const password = "fixture-password-value"
	store := inmem.NewEmailService()
	if err := store.Store("recipient@nar.cy", smtpbox.Email{Body: "captured message"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(0, store, WithBasicAuth(username, password))
	for _, route := range []struct {
		method string
		path   string
		status int
	}{
		{stdhttp.MethodGet, "/", 200},
		{stdhttp.MethodGet, "/health", 200},
		{stdhttp.MethodHead, "/health", 200},
		{stdhttp.MethodGet, "/v1/mailboxes", 200},
		{stdhttp.MethodGet, "/v1/mailboxes/recipient@nar.cy", 200},
		{stdhttp.MethodGet, "/v1/mailboxes/recipient@nar.cy/items", 200},
		{stdhttp.MethodGet, "/v1//mailboxes", 307},
		{stdhttp.MethodGet, "/unknown", 404},
		{stdhttp.MethodPost, "/health", 405},
	} {
		for _, auth := range []struct {
			name     string
			username string
			password string
			header   string
			valid    bool
		}{
			{"missing", "", "", "", false},
			{"wrong-user", "incorrect-reader", password, "", false},
			{"wrong-password", username, "incorrect-password", "", false},
			{"long-values", strings.Repeat("x", 4096), strings.Repeat("y", 4096), "", false},
			{"wrong-scheme", "", "", "Bearer invalid", false},
			{"invalid-base64", "", "", "Basic !!!!", false},
			{"missing-separator", "", "", "Basic " + base64.StdEncoding.EncodeToString([]byte("missing-separator")), false},
			{"correct", username, password, "", true},
		} {
			t.Run(route.method+route.path+"/"+auth.name, func(t *testing.T) {
				request := httptest.NewRequest(route.method, route.path, nil)
				if auth.username != "" || auth.password != "" {
					request.SetBasicAuth(auth.username, auth.password)
				} else if auth.header != "" {
					request.Header.Set("Authorization", auth.header)
				}
				response := httptest.NewRecorder()
				server.server.Handler.ServeHTTP(response, request)
				wantStatus := stdhttp.StatusUnauthorized
				if auth.valid {
					wantStatus = route.status
				}
				if response.Code != wantStatus {
					t.Errorf("expected HTTP %d, got %d", wantStatus, response.Code)
				}
				if !auth.valid {
					if response.Body.String() != `{"error":"unauthorized"}` || response.Header().Get("Content-Type") != "application/json" {
						t.Error("authentication failure must return the fixed JSON response")
					}
					if response.Header().Get("WWW-Authenticate") != `Basic realm="smtpbox"` || response.Header().Get("Location") != "" {
						t.Error("authentication must precede redirects and use the fixed challenge")
					}
				}
				if strings.Contains(response.Body.String(), username) || strings.Contains(response.Body.String(), password) {
					t.Error("response exposed authentication configuration")
				}
			})
		}
	}
}

func TestHTTPAuthenticationDisabled(t *testing.T) {
	for _, options := range [][]Option{nil, {WithBasicAuth("", "")}} {
		server := NewServer(0, inmem.NewEmailService(), options...)
		request := httptest.NewRequest(stdhttp.MethodGet, "/health", nil)
		response := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusOK || response.Header().Get("WWW-Authenticate") != "" {
			t.Error("default local API must remain available without authentication")
		}
	}
}

func TestHTTPAuthenticationAllowsPasswordColon(t *testing.T) {
	server := NewServer(0, inmem.NewEmailService(), WithBasicAuth("fixture-reader", "fixture:password"))
	request := httptest.NewRequest(stdhttp.MethodGet, "/health", nil)
	request.SetBasicAuth("fixture-reader", "fixture:password")
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK {
		t.Errorf("password containing a colon was rejected: HTTP %d", response.Code)
	}
}

func TestHTTPRejectsInvalidCredentialsBeforeBinding(t *testing.T) {
	for _, credentials := range [][2]string{{"fixture-reader", ""}, {"", "fixture-password-value"}, {"bad:reader", "fixture-password-value"}} {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		server := NewServer(port, inmem.NewEmailService(), WithBasicAuth(credentials[0], credentials[1]))
		var log bytes.Buffer
		server.log = slog.New(slog.NewTextHandler(&log, nil))
		err = server.Start()
		if err == nil {
			server.Stop()
			t.Fatal("invalid credential configuration started the HTTP server")
		}
		for _, credential := range credentials {
			if credential != "" && strings.Contains(err.Error(), credential) {
				t.Error("validation error reflected a credential value")
			}
		}
		if log.Len() != 0 {
			t.Error("invalid authentication configuration reached startup logging")
		}
		listener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			t.Fatalf("invalid authentication configuration bound the listener: %v", err)
		}
		listener.Close()
	}
}

func TestHTTPLocalAuthenticatedServe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	server := NewServer(port, inmem.NewEmailService(), WithBasicAuth("fixture-reader", "fixture-password-value"))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	client := &stdhttp.Client{Timeout: time.Second}
	for _, authenticated := range []bool{false, true} {
		request, err := stdhttp.NewRequest(stdhttp.MethodGet, "http://"+server.server.Addr+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		if authenticated {
			request.SetBasicAuth("fixture-reader", "fixture-password-value")
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		wantStatus := stdhttp.StatusUnauthorized
		if authenticated {
			wantStatus = stdhttp.StatusOK
		}
		if response.StatusCode != wantStatus {
			t.Errorf("expected HTTP %d, got %d", wantStatus, response.StatusCode)
		}
	}
	conn, err := net.DialTimeout("tcp", server.server.Addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := io.WriteString(conn, "OPTIONS * HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := stdhttp.ReadResponse(bufio.NewReader(conn), &stdhttp.Request{Method: stdhttp.MethodOptions})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != stdhttp.StatusUnauthorized {
		t.Errorf("general OPTIONS bypassed authentication: HTTP %d", response.StatusCode)
	}
}
