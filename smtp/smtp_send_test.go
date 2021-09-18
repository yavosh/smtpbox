package smtp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

func TestSendSample(t *testing.T) {

	// Set up authentication information.
	auth := sasl.NewPlainClient("", "username", "password")

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	to := []string{"recipient@example.net"}
	msg := strings.NewReader("To: recipient@example.net\r\n" +
		"Subject: sample email\r\n" +
		"\r\n" +
		"Sample email body.\r\n")
	err := smtp.SendMail(fmt.Sprintf("localhost:%d", 1025), auth, "sender@example.org", to, msg)
	if err != nil {
		t.Fatalf("could not send email %v", err)
	}
}
