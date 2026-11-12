package inmem

import "log/slog"

func clogInmem() *slog.Logger {
	return slog.With("component", "inmem")
}
