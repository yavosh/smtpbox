// Package main is a simple email sending utility, unfortunately named given the history of email servers
package main

import (
	"flag"
	"fmt"
	"github.com/emersion/go-sasl"
	"os"
	"strings"

	"github.com/emersion/go-smtp"
)

func main() {

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
		fmt.Println("Error resoling hostname using default")
		hostname = "example.com"
	}

	flag.StringVar(&smtpHost, "smtp-host", "localhost", "hostname for smtp server")
	flag.IntVar(&smtpPort, "smtp-port", 1025, "port for smtp server where to send the email")
	flag.StringVar(&emailSubject, "email-subject", "test email", "email subject")
	flag.StringVar(&emailFrom, "email-from", "sendmail@"+hostname, "email from")
	flag.StringVar(&emailRecipient, "email-recipient", "user@example.net", "email recipient, use command to supply many")
	flag.StringVar(&emailBody, "email-body", "hi from sample email", "body for the email")
	flag.StringVar(&saslUsername, "sasl-username", "username", "sasl authentication")
	flag.StringVar(&saslPassword, "sasl-password", "", "sasl password")

	flag.Parse()

	var auth sasl.Client
	if saslUsername != "" && saslPassword != "" {
		auth = sasl.NewPlainClient("", "username", "password")
	}

	recipients := strings.Split(emailRecipient, ",")
	msg := strings.NewReader(emailBody)
	if err := smtp.SendMail(fmt.Sprintf("%s:%d", smtpHost, smtpPort), auth, emailFrom, recipients, msg); err != nil {
		fmt.Printf("Error sending email: %v\n", err)
		os.Exit(-10)
	}

	fmt.Printf("Email sent to %s\n", emailRecipient)
}
