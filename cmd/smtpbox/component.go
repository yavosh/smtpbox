package main

import "log/slog"

func clogSMTPBox() *slog.Logger {
	return slog.With("component", "smtpbox")
}
