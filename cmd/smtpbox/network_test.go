package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/yavosh/smtpbox"
)

func TestNetworkCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "smtpbox")
	if output, err := exec.Command("go", "build", "-mod=vendor", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build smtpbox: %v\n%s", err, output)
	}
	const user = "network-test-user-marker"
	const password = "network-test-password-marker"
	t.Run("help", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			flags       []string
			environment map[string]string
		}{
			{"defaults", nil, nil},
			{"environment-privacy", nil, map[string]string{"HTTP_USERNAME": user, "HTTP_PASSWORD": password}},
			{"flag-privacy", []string{"-http-username", user, "-http-password", password}, nil},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, append(tc.flags, "-help")...)
				cmd.Env = storageTestEnvironment(tc.environment)
				output, err := cmd.CombinedOutput()
				if strings.Contains(string(output), user) || strings.Contains(string(output), password) {
					t.Fatal("help exposed configured authentication values")
				}
				if err != nil || !strings.Contains(string(output), "-listen-host") || !strings.Contains(string(output), "127.0.0.1") {
					t.Fatalf("expected documented loopback default: %v", err)
				}
			})
		}
	})
	t.Run("invalid-startup", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			flags       []string
			environment map[string]string
		}{
			{"username-only", []string{"-http-username", user}, nil},
			{"password-only", []string{"-http-password", password}, nil},
			{"colon-username", []string{"-http-username", user + ":invalid", "-http-password", password}, nil},
			{"empty-host", []string{"-listen-host", ""}, nil},
			{"environment-username-only", nil, map[string]string{"HTTP_USERNAME": user}},
			{"environment-password-only", nil, map[string]string{"HTTP_PASSWORD": password}},
			{"environment-colon-username", nil, map[string]string{"HTTP_USERNAME": user + ":invalid", "HTTP_PASSWORD": password}},
			{"environment-empty-host", nil, map[string]string{"LISTEN_HOST": ""}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				args := append([]string{"-http-port", "0", "-smtp-port", "0", "-dns-port", "0"}, tc.flags...)
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Env = storageTestEnvironment(tc.environment)
				output, err := cmd.CombinedOutput()
				text := string(output)
				if strings.Contains(text, user) || strings.Contains(text, password) {
					t.Fatal("configuration error exposed configured values")
				}
				if err == nil || ctx.Err() != nil || strings.Contains(text, "starting http server") ||
					strings.Contains(text, "starting dns server") || strings.Contains(text, "starting smtp server") {
					t.Fatalf("expected configuration rejection before listeners: %v", err)
				}
			})
		}
	})
	t.Run("invalid-environment-host", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "-http-port", "0", "-smtp-port", "0", "-dns-port", "0")
		cmd.Env = storageTestEnvironment(map[string]string{"LISTEN_HOST": "invalid host marker"})
		output, err := cmd.CombinedOutput()
		if err == nil || ctx.Err() != nil || strings.Contains(string(output), "started ") {
			t.Fatalf("expected malformed host rejection before a listener starts: %v", err)
		}
	})
	for _, tc := range []struct {
		name        string
		flags       []string
		environment map[string]string
		auth        bool
	}{
		{"defaults", nil, nil, false},
		{"environment-auth", nil, map[string]string{"HTTP_USERNAME": user, "HTTP_PASSWORD": password}, true},
		{"flags-override-environment", []string{"-listen-host", "127.0.0.1", "-http-username", user, "-http-password", password}, map[string]string{"LISTEN_HOST": "invalid host marker", "HTTP_USERNAME": "overridden-user", "HTTP_PASSWORD": "overridden-password"}, true},
		{"flags-disable-environment-auth", []string{"-http-username", "", "-http-password", ""}, map[string]string{"HTTP_USERNAME": user, "HTTP_PASSWORD": password}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, smtpClient := startNetworkCLI(t, binary, tc.flags, tc.environment)
			transport := &http.Transport{Proxy: nil}
			t.Cleanup(transport.CloseIdleConnections)
			httpClient := &http.Client{Transport: transport, Timeout: time.Second}
			request := func(path, username, secret string) *http.Response {
				t.Helper()
				req, err := http.NewRequest(http.MethodGet, base+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				if username != "" || secret != "" {
					req.SetBasicAuth(username, secret)
				}
				response, err := httpClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				return response
			}
			response := request("/health", "", "")
			wantStatus := http.StatusOK
			if tc.auth {
				wantStatus = http.StatusUnauthorized
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != wantStatus {
				t.Fatalf("unauthenticated health status=%d, expected=%d", response.StatusCode, wantStatus)
			}
			if tc.auth {
				for _, pair := range [][2]string{{user, "wrong-password"}, {"wrong-user", password}, {"overridden-user", "overridden-password"}} {
					response := request("/health", pair[0], pair[1])
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
					if response.StatusCode != http.StatusUnauthorized {
						t.Fatalf("incorrect authentication accepted: status=%d", response.StatusCode)
					}
				}
			}
			body := "Subject: configured authentication\r\n\r\nstored through SMTP\r\n"
			if err := smtpClient.SendMail("sender@nar.cy", []string{"recipient@nar.cy"}, strings.NewReader(body)); err != nil {
				t.Fatal(err)
			}
			username, secret := "", ""
			if tc.auth {
				username, secret = user, password
			}
			response = request("/v1/mailboxes/recipient@nar.cy/items", username, secret)
			defer response.Body.Close()
			var result struct {
				Result []smtpbox.Email `json:"result"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || len(result.Result) != 1 || result.Result[0].Body != body {
				t.Fatalf("authenticated message retrieval failed: status=%d count=%d", response.StatusCode, len(result.Result))
			}
		})
	}
}

func startNetworkCLI(t *testing.T, binary string, flags []string, environment map[string]string) (string, *gosmtp.Client) {
	t.Helper()
	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	smtpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		httpListener.Close()
		t.Fatal(err)
	}
	httpPort := httpListener.Addr().(*net.TCPAddr).Port
	smtpPort := smtpListener.Addr().(*net.TCPAddr).Port
	httpListener.Close()
	smtpListener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	args := []string{"-http-port", strconv.Itoa(httpPort), "-smtp-port", strconv.Itoa(smtpPort), "-dns-port", "0", "-smtp-domain", "nar.cy", "-dns-domain", "nar.cy"}
	cmd := exec.CommandContext(ctx, binary, append(args, flags...)...)
	cmd.Env = storageTestEnvironment(environment)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGTERM)
		if err := cmd.Wait(); err != nil {
			t.Errorf("smtpbox did not exit cleanly: %v", err)
		}
	})
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(smtpPort))
	deadline := time.Now().Add(5 * time.Second)
	for {
		client, err := gosmtp.Dial(address)
		if err == nil {
			client.CommandTimeout = time.Second
			client.SubmissionTimeout = time.Second
			if err = client.Noop(); err == nil {
				t.Cleanup(func() { client.Close() })
				return fmt.Sprintf("http://127.0.0.1:%d", httpPort), client
			}
			client.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for SMTP readiness: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
