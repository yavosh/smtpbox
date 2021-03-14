package smtp_test

import (
	"fmt"
	"github.com/phayes/freeport"
	"log"
	"strings"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	localsmtp "github.com/yavosh/smtpmocker/agents/smtp"
)

func TestSendEmail(t *testing.T) {

	port, err := freeport.GetFreePort()
	if err != nil {
		t.Fatalf("could not allocate port %v", err)
	}

	log.Printf("using local port %d", port)
	localServer := localsmtp.NewServer("localhost", localsmtp.WithListenAddr(fmt.Sprintf(":%d", port)))
	localServer.Start()

	// Set up authentication information.
	auth := sasl.NewPlainClient("", "username", "password")

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	to := []string{"recipient@example.net"}
	msg := strings.NewReader("To: recipient@example.net\r\n" +
		"Subject: discount Gophers!\r\n" +
		"\r\n" +
		"This is the email body.\r\n")
	err = smtp.SendMail(fmt.Sprintf("localhost:%d", port), auth, "sender@example.org", to, msg)
	if err != nil {
		t.Fatalf("could not send email %v", err)
	}

	if err := localServer.Stop(); err != nil {
		t.Fatalf("cloud not stop server %v", err)
	}
}
