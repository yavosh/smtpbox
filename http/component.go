package http

import "log/slog"

func clogHTTP() *slog.Logger {
	return slog.With("component", "http")
}
