package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/yavosh/smtpbox/dns"
	"github.com/yavosh/smtpbox/http"
	"github.com/yavosh/smtpbox/inmem"
	"github.com/yavosh/smtpbox/smtp"
)

func main() {
	var (
		httpPort           int
		smtpPort           int
		smtpDomain         string
		dnsPort            int
		dnsDomain          string
		maxMessagesText    string
		maxMemoryBytesText string
	)

	flag.IntVar(&httpPort, "http-port", envInt("HTTP_PORT", 8080), "listen port for the http server")
	flag.IntVar(&smtpPort, "smtp-port", envInt("SMTP_PORT", 1025), "listen port for the smtp server")
	flag.StringVar(&smtpDomain, "smtp-domain", envString("SMTP_DOMAIN", "localhost"), "domain for smtp server")
	flag.IntVar(&dnsPort, "dns-port", envInt("DNS_PORT", 1053), "listen port for the dns server")
	flag.StringVar(&dnsDomain, "dns-domain", envString("DNS_DOMAIN", "localhost"), "domain for dns server")
	flag.StringVar(&maxMessagesText, "max-messages", envLimit("MAX_MESSAGES", strconv.Itoa(inmem.DefaultMaxMessages)), "maximum retained recipient copies")
	flag.StringVar(&maxMemoryBytesText, "max-memory-bytes", envLimit("MAX_MEMORY_BYTES", strconv.FormatInt(inmem.DefaultMaxMemoryBytes, 10)), "maximum accounted storage bytes")
	flag.Parse()

	maxMessages, maxMemoryBytes, err := parseStorageLimits(maxMessagesText, maxMemoryBytesText)
	if err != nil {
		slog.Error("invalid storage limits", "error", err)
		os.Exit(1)
	}
	backend, err := inmem.NewEmailServiceWithLimits(maxMessages, maxMemoryBytes)
	if err != nil {
		slog.Error("invalid storage limits", "error", err)
		os.Exit(1)
	}

	smtpServer := smtp.NewServer(
		fmt.Sprintf(":%d", smtpPort),
		smtpDomain,
		backend,
	)

	httpServer := http.NewServer(
		httpPort,
		backend,
	)

	dnsServer := dns.NewServer(
		dnsPort,
		dnsDomain,
	)

	if err := httpServer.Start(); err != nil {
		slog.Error("error starting http", "error", err)
		os.Exit(1)
	}

	if err := dnsServer.Start(); err != nil {
		slog.Error("error starting dns", "error", err)
		os.Exit(1)
	}

	if err := smtpServer.Start(); err != nil {
		slog.Error("error starting smtp", "error", err)
		os.Exit(1)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	if err := smtpServer.Stop(); err != nil {
		slog.Error("error stopping smtp", "error", err)
	}

	if err := httpServer.Stop(); err != nil {
		slog.Error("error stopping http", "error", err)
	}

	if err := dnsServer.Stop(); err != nil {
		slog.Error("error stopping dns", "error", err)
	}
}

func parseStorageLimits(maxMessagesText, maxMemoryBytesText string) (int, int64, error) {
	maxMessages, err := strconv.ParseInt(maxMessagesText, 10, strconv.IntSize)
	if err != nil || maxMessages <= 0 {
		return 0, 0, errors.New("max-messages must be a positive integer")
	}
	maxMemoryBytes, err := strconv.ParseInt(maxMemoryBytesText, 10, 64)
	if err != nil || maxMemoryBytes <= 0 {
		return 0, 0, errors.New("max-memory-bytes must be a positive integer")
	}
	return int(maxMessages), maxMemoryBytes, nil
}

func envLimit(key, fallback string) string {
	if value, present := os.LookupEnv(key); present {
		return value
	}
	return fallback
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
