package dns

import "log/slog"

func clogDNS() *slog.Logger { return slog.With("component", "dns") }
