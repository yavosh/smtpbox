// Package main is a simple email sending utility, unfortunately named given the history of email servers
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error sending email: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var (
		smtpHost       string
		smtpPort       int
		emailSubject   string
		emailFrom      string
		emailRecipient string
		emailBody      string
		saslUsername   string
		saslPassword   string
	)

	hostname, err := os.Hostname()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Error resolving hostname; using default")
		hostname = "example.com"
	}

	flags := flag.NewFlagSet("sendmail", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&smtpHost, "smtp-host", "localhost", "hostname for smtp server")
	flags.IntVar(&smtpPort, "smtp-port", 1025, "port for smtp server where to send the email")
	flags.StringVar(&emailSubject, "email-subject", "", "optional email subject override")
	flags.StringVar(&emailFrom, "email-from", "sendmail@"+hostname, "email from")
	flags.StringVar(&emailRecipient, "email-recipient", "user@localhost", "email recipient, use commas to supply many")
	flags.StringVar(&emailBody, "email-body", "hi from sample email", "raw email message or body")
	flags.StringVar(&saslUsername, "sasl-username", "username", "sasl authentication")
	flags.StringVar(&saslPassword, "sasl-password", "", "sasl password")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	message, err := withSubject(emailBody, emailSubject)
	if err != nil {
		return err
	}
	var auth sasl.Client
	if saslUsername != "" && saslPassword != "" {
		auth = sasl.NewPlainClient("", saslUsername, saslPassword)
	}
	address := net.JoinHostPort(smtpHost, strconv.Itoa(smtpPort))
	if err := sendMail(ctx, address, auth, emailFrom, strings.Split(emailRecipient, ","), strings.NewReader(message)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Email sent to %s\n", emailRecipient)
	return err
}

func withSubject(message, subject string) (string, error) {
	if strings.ContainsAny(subject, "\r\n") {
		return "", errors.New("email subject must not contain CR or LF")
	}
	if subject == "" {
		return message, nil
	}
	subject = mime.QEncoding.Encode("utf-8", subject)
	newline := "\r\n"
	separator := strings.Index(message, "\r\n\r\n")
	if lfSeparator := strings.Index(message, "\n\n"); lfSeparator >= 0 && (separator < 0 || lfSeparator < separator) {
		newline = "\n"
		separator = lfSeparator
	}
	validHeaders := false
	if separator >= 0 {
		_, err := mail.ReadMessage(strings.NewReader(message))
		validHeaders = err == nil
	}
	if !validHeaders {
		return "Subject: " + subject + "\r\n\r\n" + message, nil
	}

	// Copy header lines verbatim, replacing Subject and its continuation lines.
	var result strings.Builder
	found, skip := false, false
	for line := range strings.SplitAfterSeq(message[:separator+len(newline)], "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if !skip {
				result.WriteString(line)
			}
			continue
		}
		name, _, _ := strings.Cut(line, ":")
		skip = strings.EqualFold(name, "Subject")
		if skip {
			if !found {
				result.WriteString("Subject: " + subject + newline)
				found = true
			}
		} else {
			result.WriteString(line)
		}
	}
	result.WriteString(message[separator+len(newline):])
	if !found {
		return "Subject: " + subject + newline + result.String(), nil
	}
	return result.String(), nil
}

func sendMail(ctx context.Context, address string, auth sasl.Client, from string, recipients []string, message io.Reader) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	dialer := net.Dialer{Timeout: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client := smtp.NewClient(conn)
	defer func() {
		if closeErr := client.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
	}()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.SendMail(from, recipients, message); err != nil {
		return err
	}
	return client.Quit()
}
