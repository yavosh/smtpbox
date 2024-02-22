package smtp_test

import (
	"fmt"
	"log"
	"strings"
	"testing"

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

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	to := []string{"recipient@example.net"}
	msg := strings.NewReader("To: recipient@example.net\r\n" +
		"Subject: discount Gophers!\r\n" +
		"\r\n" +
		"This is the email body.\r\n")

	smtpClient, err := smtp.Dial(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		t.Fatalf("could not dial %v", err)
	}

	if err := smtpClient.Hello("localhost"); err != nil {
		t.Fatalf("could not hello %v", err)
	}

	// Set up authentication information.
	//auth := sasl.NewPlainClient("test", "username", "password")
	//if err := smtpClient.Auth(auth); err != nil {
	//	t.Fatalf("could not auth %v", err)
	//}

	if err := smtpClient.SendMail("sender@example.net", to, msg); err != nil {
		t.Fatalf("could not send %v", err)
	}

	//err = smtp.SendMail(fmt.Sprintf("localhost:%d", port), nil, "sender@example.org", to, msg)
	//if err != nil {
	//	t.Fatalf("could not send email %v", err)
	//}

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
