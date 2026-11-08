package logs

import (
	"context"
	"log/slog"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
)

// SinkHandler implements slog.Handler, forwarding records to an events.Sink.
type SinkHandler struct {
	sink  events.Sink
	level slog.Level
}

// NewSinkHandler creates a handler that writes to sink at the given level.
func NewSinkHandler(sink events.Sink, level slog.Level) *SinkHandler {
	if sink == nil {
		sink = events.NopSink{}
	}
	return &SinkHandler{sink: sink, level: level}
}

// Enabled reports whether the handler accepts records at level l.
func (h *SinkHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

// Handle forwards the record to the sink.
func (h *SinkHandler) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	if msg == "" {
		return nil
	}
	e := events.Log{Text: msg}
	switch {
	case r.Level >= slog.LevelError:
		e.Level = events.LevelError
	case r.Level >= slog.LevelWarn:
		e.Level = events.LevelWarning
	case r.Level >= slog.LevelInfo:
		e.Level = events.LevelInfo
	default:
		e.Level = events.LevelDebug
	}
	h.sink.Log(e)
	return nil
}

// WithAttrs implements slog.Handler.
func (h *SinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Attributes are dropped for now; not needed for current CLI logs.
	return h
}

// WithGroup implements slog.Handler.
func (h *SinkHandler) WithGroup(name string) slog.Handler {
	_ = name
	return h
}
