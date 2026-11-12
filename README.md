# smtpbox

[![Go Tests](https://github.com/yavosh/smtpbox/actions/workflows/go.yml/badge.svg)](https://github.com/yavosh/smtpbox/actions/workflows/go.yml)

A lightweight SMTP server for development and testing. Send emails via SMTP and retrieve them through a REST API.

Perfect for integration tests that need to verify email delivery — welcome emails, password resets, verification links, etc.

## Quick Start

### Run with Go

```bash
go build -o bin/smtpbox ./cmd/smtpbox
./bin/smtpbox
```

### Run with Docker

```bash
docker compose up --build
```

The server starts three services:

| Service | Default Port | Protocol |
|---------|-------------|----------|
| HTTP API | 8080 | TCP |
| SMTP | 1025 | TCP |
| DNS | 1053 | UDP |

SMTP accepts messages up to 1 MiB (1,048,576 bytes), including headers.
Larger messages receive SMTP error `552` and are not stored.
Recipient domains must match `-smtp-domain` exactly, ignoring case.
Other domains receive SMTP error `550` during `RCPT TO`.
Storage failures return SMTP error `451` instead of reporting successful delivery.
Earlier recipient copies can remain stored after a later storage failure, so retries can create duplicates.

## Configuration

All options can be set via flags or environment variables.

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-http-port` | `HTTP_PORT` | `8080` | HTTP API listen port |
| `-smtp-port` | `SMTP_PORT` | `1025` | SMTP server listen port |
| `-smtp-domain` | `SMTP_DOMAIN` | `localhost` | Domain the SMTP server accepts mail for |
| `-dns-port` | `DNS_PORT` | `1053` | DNS server listen port |
| `-dns-domain` | `DNS_DOMAIN` | `localhost` | Domain the DNS server responds for |

```bash
# Using flags
./bin/smtpbox -smtp-port 2525 -smtp-domain example.net

# Using environment variables
SMTP_PORT=2525 SMTP_DOMAIN=example.net ./bin/smtpbox
```

With `-dns-domain nar.cy`, DNS returns MX target `mail.nar.cy.`.
A queries for `nar.cy.` and `mail.nar.cy.` return `127.0.0.1`.
Names ignore case and accept a trailing dot.
Unknown names inside the configured domain return `NXDOMAIN`; unrelated names return `REFUSED`.
Existing names queried for unsupported record types return an empty successful response.
DNS supports only `IN` queries.

## Usage

### 1. Send an email via SMTP

The included `sendmail` utility uses plaintext SMTP for local development.
Optional `-sasl-username` and `-sasl-password` flags send test credentials without encryption.
SMTPBox accepts any test credentials.

```bash
go run ./cmd/sendmail -smtp-port 1025 \
  -email-from sender@example.net \
  -email-recipient user@localhost \
  -email-body "Hello from smtpbox"
```

Or with `curl` and any SMTP client:

```bash
# Using swaks
swaks --to user@localhost --from sender@example.net \
  --server localhost:1025 --body "Test email"

# Using Python
python3 -c "
import smtplib
from email.message import EmailMessage
msg = EmailMessage()
msg['From'] = 'sender@example.net'
msg['To'] = 'user@localhost'
msg['Subject'] = 'Test'
msg.set_content('Hello from smtpbox')
with smtplib.SMTP('localhost', 1025) as s:
    s.send_message(msg)
"
```

### 2. Retrieve emails via the HTTP API

```bash
# List all mailboxes
curl http://localhost:8080/v1/mailboxes

# Get mailbox details
curl http://localhost:8080/v1/mailboxes/user@localhost

# List emails in a mailbox
curl http://localhost:8080/v1/mailboxes/user@localhost/items
```

## API Reference

### `GET /`

Returns service info.

```json
{"result": {"name": "smtpbox api service"}}
```

### `GET /health`

Health check endpoint.

```json
{"result": {"status": "ok"}}
```

### `GET /v1/mailboxes`

List all mailboxes that have received email.

```json
{"result": ["user@localhost", "admin@localhost"]}
```

### `GET /v1/mailboxes/{address}`

Get details for a specific mailbox.

```json
{"result": {"Addr": "user@localhost", "Size": 2, "CreatedAt": "2024-01-15T10:30:00Z"}}
```

### `GET /v1/mailboxes/{address}/items`

List all emails in a mailbox.

```json
{
  "result": [
    {
      "From": "sender@example.net",
      "To": ["user@localhost"],
      "Body": "Subject: Test\r\n\r\nHello from smtpbox",
      "Received": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Error Responses

```json
{"error": "not found"}      // 404
{"error": "internal server error"}  // 500
```

## Integration Test Example (Go)

`Start` returns after the SMTP listener is ready and reports unavailable listen addresses as errors.

The in-memory backend copies recipient slices when storing messages.
Each `List` call returns an independent snapshot, including recipient slices.
Changing input slices or snapshots does not change stored messages or other snapshots.

```go
func TestWelcomeEmail(t *testing.T) {
    // Start smtpbox
    backend := inmem.NewEmailService()
    srv := smtp.NewServer(":1025", "localhost", backend)
    if err := srv.Start(); err != nil {
        t.Fatal(err)
    }
    defer srv.Stop()

    // ... trigger your app to send a welcome email to user@localhost ...

    // Verify the email was sent
    emails, err := backend.List("user@localhost")
    if err != nil {
        t.Fatal(err)
    }
    if len(emails) != 1 {
        t.Fatalf("expected 1 email, got %d", len(emails))
    }
    if !strings.Contains(emails[0].Body, "Welcome") {
        t.Error("expected welcome email")
    }
}
```

## Development

The vendored SMTP library corrects message size boundaries, malformed domains, and connection handling during shutdown.
Preserve these corrections when updating dependencies, and build with vendored dependencies.
The `-mod=mod` build option bypasses these corrections.

```bash
make build     # Build binaries
make test      # Run tests
make fmt       # Format code
make lint      # Lint code
make up        # Run with Docker Compose
make down      # Stop Docker Compose
```

## License

MIT
