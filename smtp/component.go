package smtp

import "log/slog"

func clogSMTP() *slog.Logger {
	return slog.With("component", "smtp")
}
