package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/yavosh/smtpbox/dns"
	"github.com/yavosh/smtpbox/http"
	"github.com/yavosh/smtpbox/inmem"
	"github.com/yavosh/smtpbox/smtp"
)

type config struct {
	httpPort       int
	smtpPort       int
	smtpDomain     string
	dnsPort        int
	dnsDomain      string
	maxMessages    int
	maxMemoryBytes int64
	listenHost     string
	httpUsername   string
	httpPassword   string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.LookupEnv, os.Stderr, clogSMTPBox())
	stop()
	if err != nil {
		clogSMTPBox().Error("smtpbox stopped", "error", err)
		os.Exit(1)
	}
}

func parseConfig(args []string, lookupEnv func(string) (string, bool), output io.Writer) (config, error) {
	envValue := func(key, fallback string) string {
		if value, present := lookupEnv(key); present {
			return value
		}
		return fallback
	}
	envString := func(key, fallback string) string {
		if value := envValue(key, fallback); value != "" {
			return value
		}
		return fallback
	}
	envInt := func(key string, fallback int) int {
		if value, err := strconv.Atoi(envValue(key, "")); err == nil {
			return value
		}
		return fallback
	}

	var cfg config
	var maxMessagesText, maxMemoryBytesText string
	flags := flag.NewFlagSet("smtpbox", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.IntVar(&cfg.httpPort, "http-port", envInt("HTTP_PORT", 8080), "listen port for the http server")
	flags.IntVar(&cfg.smtpPort, "smtp-port", envInt("SMTP_PORT", 1025), "listen port for the smtp server")
	flags.StringVar(&cfg.smtpDomain, "smtp-domain", envString("SMTP_DOMAIN", "localhost"), "domain for smtp server")
	flags.IntVar(&cfg.dnsPort, "dns-port", envInt("DNS_PORT", 1053), "listen port for the dns server")
	flags.StringVar(&cfg.dnsDomain, "dns-domain", envString("DNS_DOMAIN", "localhost"), "domain for dns server")
	flags.StringVar(&maxMessagesText, "max-messages", envValue("MAX_MESSAGES", strconv.Itoa(inmem.DefaultMaxMessages)), "maximum retained recipient copies")
	flags.StringVar(&maxMemoryBytesText, "max-memory-bytes", envValue("MAX_MEMORY_BYTES", strconv.FormatInt(inmem.DefaultMaxMemoryBytes, 10)), "maximum accounted storage bytes")
	flags.StringVar(&cfg.listenHost, "listen-host", envValue("LISTEN_HOST", "127.0.0.1"), "listen host for all servers")
	flags.StringVar(&cfg.httpUsername, "http-username", envString("HTTP_USERNAME", ""), "HTTP Basic authentication username")
	flags.StringVar(&cfg.httpPassword, "http-password", envString("HTTP_PASSWORD", ""), "HTTP Basic authentication password")
	flags.Lookup("http-username").DefValue = ""
	flags.Lookup("http-password").DefValue = ""
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.listenHost == "" {
		return config{}, errors.New("listen host must not be empty")
	}
	var err error
	cfg.maxMessages, cfg.maxMemoryBytes, err = parseStorageLimits(maxMessagesText, maxMemoryBytesText)
	if err != nil {
		return config{}, fmt.Errorf("invalid storage limits: %w", err)
	}
	return cfg, nil
}

func run(ctx context.Context, args []string, lookupEnv func(string) (string, bool), output io.Writer, log *slog.Logger) (result error) {
	cfg, err := parseConfig(args, lookupEnv, output)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	backend, err := inmem.NewEmailServiceWithLimits(cfg.maxMessages, cfg.maxMemoryBytes)
	if err != nil {
		return fmt.Errorf("invalid storage limits: %w", err)
	}
	httpServer := http.NewServer(cfg.httpPort, backend,
		http.WithListenHost(cfg.listenHost), http.WithBasicAuth(cfg.httpUsername, cfg.httpPassword))
	if err := httpServer.StartContext(ctx); err != nil {
		return fmt.Errorf("start http: %w", err)
	}
	defer func() {
		if err := httpServer.Stop(); err != nil {
			result = errors.Join(result, fmt.Errorf("stop http: %w", err))
		}
	}()

	dnsServer := dns.NewServer(cfg.dnsPort, cfg.dnsDomain, dns.WithListenHost(cfg.listenHost))
	if err := dnsServer.StartContext(ctx); err != nil {
		return fmt.Errorf("start dns: %w", err)
	}
	defer func() {
		if err := dnsServer.Stop(); err != nil {
			result = errors.Join(result, fmt.Errorf("stop dns: %w", err))
		}
	}()

	smtpServer := smtp.NewServer(net.JoinHostPort(cfg.listenHost, strconv.Itoa(cfg.smtpPort)), cfg.smtpDomain, backend)
	if err := smtpServer.StartContext(ctx); err != nil {
		return fmt.Errorf("start smtp: %w", err)
	}
	defer func() {
		if err := smtpServer.Stop(); err != nil {
			result = errors.Join(result, fmt.Errorf("stop smtp: %w", err))
		}
	}()

	log.Info("smtpbox ready", "http_addr", httpServer.Addr().String(), "smtp_addr", smtpServer.Addr().String(), "dns_addr", dnsServer.Addr().String())
	<-ctx.Done()
	return nil
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
