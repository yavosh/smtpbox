package email

import "time"

type Email struct {
	From     string
	To       []string
	Body     string
	Received time.Time
}

type Mailbox struct {
	Addr      string
	Size      int
	CreatedAt time.Time
}
