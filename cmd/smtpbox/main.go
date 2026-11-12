package main

import (
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
		httpPort   int
		smtpPort   int
		smtpDomain string
		dnsPort    int
		dnsDomain  string
	)

	flag.IntVar(&httpPort, "http-port", envInt("HTTP_PORT", 8080), "listen port for the http server")
	flag.IntVar(&smtpPort, "smtp-port", envInt("SMTP_PORT", 1025), "listen port for the smtp server")
	flag.StringVar(&smtpDomain, "smtp-domain", envString("SMTP_DOMAIN", "localhost"), "domain for smtp server")
	flag.IntVar(&dnsPort, "dns-port", envInt("DNS_PORT", 1053), "listen port for the dns server")
	flag.StringVar(&dnsDomain, "dns-domain", envString("DNS_DOMAIN", "localhost"), "domain for dns server")
	flag.Parse()

	backend := inmem.NewEmailService()

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
