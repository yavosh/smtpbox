package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func testSignalShutdown(t *testing.T, binary string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	logs, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	defer writer.Close()
	cmd := exec.CommandContext(ctx, binary, "-http-port", "0", "-smtp-port", "0", "-dns-port", "0")
	cmd.Env = storageTestEnvironment(nil)
	cmd.Stdout, cmd.Stderr = writer, writer
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	ready := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(logs)
		announced := false
		for scanner.Scan() {
			if !announced && strings.Contains(scanner.Text(), "smtpbox ready") {
				close(ready)
				announced = true
			}
		}
		readDone <- scanner.Err()
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	waited := false
	defer func() {
		cancel()
		if !waited {
			<-exited
		}
		logs.Close()
		<-readDone
	}()

	select {
	case <-ready:
	case err := <-exited:
		waited = true
		t.Fatalf("smtpbox exited before readiness: %v", err)
	case <-ctx.Done():
		t.Fatalf("wait for smtpbox readiness: %v", ctx.Err())
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := <-exited; err != nil {
		waited = true
		t.Fatalf("smtpbox shutdown: %v", err)
	}
	waited = true
	if ctx.Err() != nil {
		t.Fatalf("smtpbox shutdown exceeded deadline: %v", ctx.Err())
	}
}
