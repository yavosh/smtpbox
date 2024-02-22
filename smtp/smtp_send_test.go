package smtp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-smtp"
)

func TestSendSample(t *testing.T) {
	t.Skip("use with running server only")

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	to := []string{"recipient@example.net"}
	msg := strings.NewReader("To: recipient@example.net\r\n" +
		"Subject: sample email\r\n" +
		"\r\n" +
		"Sample email body.\r\n")

	smtpClient, err := smtp.Dial(fmt.Sprintf("localhost:%d", 1025))
	if err != nil {
		t.Fatalf("could not dial %v", err)
	}

	if err := smtpClient.Hello("localhost"); err != nil {
		t.Fatalf("could not hello %v", err)
	}

	if err := smtpClient.SendMail("sender@example.net", to, msg); err != nil {
		t.Fatalf("could not send %v", err)
	}
}
