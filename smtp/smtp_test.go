package smtp_test

import (
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/phayes/freeport"
	"github.com/yavosh/smtpbox/inmem"
	s "github.com/yavosh/smtpbox/smtp"
)

func TestSendEmail(t *testing.T) {
	port, err := freeport.GetFreePort()
	if err != nil {
		t.Fatalf("could not allocate port %v", err)
	}

	backend := inmem.NewEmailService()

	log.Printf("using local port %d", port)
	localServer := s.NewServer(fmt.Sprintf(":%d", port), "example.net", backend)
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

	mb, err := backend.GetMailbox("recipient@example.net")
	if err != nil {
		t.Fatalf("error %v", err)
	}

	fmt.Printf("MB %+v\n", mb)

	emails, err := backend.List("recipient@example.net")
	if err != nil {
		t.Fatalf("error %v", err)
	}

	fmt.Printf("emails %+v\n", emails)
}
