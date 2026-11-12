package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yavosh/smtpbox"
)

func TestParseStorageLimits(t *testing.T) {
	count, budget, err := parseStorageLimits("2", "4096")
	if err != nil || count != 2 || budget != 4096 {
		t.Fatalf("incorrect limits: %d %d %v", count, budget, err)
	}
	for _, value := range []string{"", "0", "-1", "not-a-number", "1.5", " 1", "9223372036854775808", "18446744073709551616"} {
		if _, _, err := parseStorageLimits(value, "4096"); err == nil {
			t.Errorf("invalid message count accepted: %q", value)
		} else if err.Error() != "max-messages must be a positive integer" {
			t.Errorf("unexpected message count error: %v", err)
		}
		if _, _, err := parseStorageLimits("2", value); err == nil {
			t.Errorf("invalid memory budget accepted: %q", value)
		} else if err.Error() != "max-memory-bytes must be a positive integer" {
			t.Errorf("unexpected memory budget error: %v", err)
		}
	}
}

func storageTestEnvironment(values map[string]string) []string {
	var environment []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "MAX_MESSAGES=") && !strings.HasPrefix(value, "MAX_MEMORY_BYTES=") &&
			!strings.HasPrefix(value, "LISTEN_HOST=") && !strings.HasPrefix(value, "HTTP_USERNAME=") && !strings.HasPrefix(value, "HTTP_PASSWORD=") {
			environment = append(environment, value)
		}
	}
	for name, value := range values {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func TestStorageLimitCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "smtpbox")
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancelBuild)
	if output, err := exec.CommandContext(buildCtx, "go", "build", "-mod=vendor", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build smtpbox: %v\n%s", err, output)
	}
	t.Run("invalid-startup", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			flags       []string
			environment map[string]string
		}{
			{"zero-count", []string{"-max-messages", "0"}, nil},
			{"negative-count", []string{"-max-messages", "-1"}, nil},
			{"overflow-count", []string{"-max-messages", "9223372036854775808"}, nil},
			{"zero-budget", []string{"-max-memory-bytes", "0"}, nil},
			{"negative-budget", []string{"-max-memory-bytes", "-1"}, nil},
			{"overflow-budget", []string{"-max-memory-bytes", "9223372036854775808"}, nil},
			{"fixed-allocation", []string{"-max-messages", "1000", "-max-memory-bytes", "1"}, nil},
			{"allocation-overflow", []string{"-max-messages", "9223372036854775807", "-max-memory-bytes", "9223372036854775807"}, nil},
			{"invalid-count-env", nil, map[string]string{"MAX_MESSAGES": "not-a-number"}},
			{"invalid-budget-env", nil, map[string]string{"MAX_MEMORY_BYTES": "not-a-number"}},
			{"empty-count-env", nil, map[string]string{"MAX_MESSAGES": ""}},
			{"empty-budget-env", nil, map[string]string{"MAX_MEMORY_BYTES": ""}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				args := append([]string{"-http-port", "0", "-smtp-port", "0", "-dns-port", "0"}, tc.flags...)
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Env = storageTestEnvironment(tc.environment)
				output, err := cmd.CombinedOutput()
				if err == nil || ctx.Err() != nil || !strings.Contains(string(output), "invalid storage limits") || strings.Contains(string(output), "starting ") {
					t.Fatalf("expected validation before listeners: %v\n%s", err, output)
				}
				if strings.Contains(string(output), "not-a-number") {
					t.Error("validation error echoed the configured environment value")
				}
			})
		}
	})
	for _, tc := range []struct {
		name        string
		flags       []string
		environment map[string]string
		wantCount   int
	}{
		{"environment", nil, map[string]string{"MAX_MESSAGES": "2", "MAX_MEMORY_BYTES": "4096"}, 2},
		{"environment-memory-ceiling", nil, map[string]string{"MAX_MESSAGES": "3", "MAX_MEMORY_BYTES": "1000"}, 1},
		{"flags-override-invalid-environment", []string{"-max-messages", "2", "-max-memory-bytes", "4096"}, map[string]string{"MAX_MESSAGES": "not-a-number", "MAX_MEMORY_BYTES": "not-a-number"}, 2},
		{"flags-override-empty-environment", []string{"-max-messages", "2", "-max-memory-bytes", "4096"}, map[string]string{"MAX_MESSAGES": "", "MAX_MEMORY_BYTES": ""}, 2},
		{"memory-ceiling", []string{"-max-messages", "3", "-max-memory-bytes", "1000"}, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, client := startNetworkApp(t, tc.flags, tc.environment)
			var latest string
			for i := range 3 {
				latest = fmt.Sprint(i) + strings.Repeat("x", 397) + "\r\n"
				if err := client.SendMail("sender@nar.cy", []string{"recipient@nar.cy"}, strings.NewReader(latest)); err != nil {
					t.Fatal(err)
				}
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/v1/mailboxes/recipient@nar.cy/items", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := (&http.Client{Timeout: time.Second}).Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var result struct {
				Result []smtpbox.Email `json:"result"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || len(result.Result) != tc.wantCount || result.Result[len(result.Result)-1].Body != latest {
				t.Errorf("incorrect configured retention: status=%d count=%d expected=%d", response.StatusCode, len(result.Result), tc.wantCount)
			}
		})
	}
}
