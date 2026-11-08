package core

import (
	"context"
	"log/slog"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
)

// sinkHandler adapts an events.Sink to slog.Handler.
type sinkHandler struct {
	sink  events.Sink
	level slog.Level
}

// newSinkHandler returns a handler forwarding to sink.
func newSinkHandler(sink events.Sink, level slog.Level) *sinkHandler {
	if sink == nil {
		sink = events.NopSink{}
	}
	return &sinkHandler{sink: sink, level: level}
}

func (h *sinkHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *sinkHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "" {
		return nil
	}
	var e events.Log
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
	e.Text = r.Message
	h.sink.Log(e)
	return nil
}

func (h *sinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	_ = attrs
	return h
}

func (h *sinkHandler) WithGroup(name string) slog.Handler {
	_ = name
	return h
}
