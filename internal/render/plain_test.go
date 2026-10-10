package render

import (
	"bytes"
	"testing"
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func newTestSink() (*PlainSink, *bytes.Buffer) {
	var buf bytes.Buffer
	s := &PlainSink{Out: &buf, Err: &buf}
	return s, &buf
}

func TestPlainSinkLogLevels(t *testing.T) {
	s, buf := newTestSink()
	s.Log(events.Log{Level: events.LevelInfo, Text: "hello"})
	s.Log(events.Log{Level: events.LevelWarning, Text: "careful"})
	s.Log(events.Log{Level: events.LevelError, Text: "boom"})
	s.Log(events.Log{Level: events.LevelDebug, Text: "hidden"})

	got := buf.String()
	want := "hello\nWARNING: careful\nERROR: boom\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPlainSinkQuietSuppresses(t *testing.T) {
	s, buf := newTestSink()
	s.Quiet = true
	s.Log(events.Log{Level: events.LevelInfo, Text: "hello"})
	s.Log(events.Log{Level: events.LevelWarning, Text: "careful"})
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
	// Errors are always shown.
	s.Log(events.Log{Level: events.LevelError, Text: "boom"})
	if buf.String() != "ERROR: boom\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestPlainSinkVerboseDebug(t *testing.T) {
	s, buf := newTestSink()
	s.Verbose = true
	s.Log(events.Log{Level: events.LevelDebug, Text: "trace"})
	if buf.String() != "[debug] trace\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestPlainSinkProgress(t *testing.T) {
	s, buf := newTestSink()
	s.Progress(events.Progress{Filename: "out.mp4", Status: events.StatusStarted})
	s.Progress(events.Progress{
		Filename: "out.mp4", Status: events.StatusFinished,
		Downloaded: 2 * 1024 * 1024, Elapsed: 2 * time.Second,
	})
	got := buf.String()
	want := "[download] Destination: out.mp4\n" +
		"[download] 100% of 2.00MiB in 00:02 at 1024.00KiB/s\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPlainSinkNoProgress(t *testing.T) {
	s, buf := newTestSink()
	s.NoProgress = true
	s.Progress(events.Progress{Filename: "out.mp4", Status: events.StatusStarted})
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}

func TestNewPlainSinkFromOptions(t *testing.T) {
	o := options.DefaultOptions()
	o.Quiet = true
	o.NoWarnings = true
	o.NoProgress = true
	s := NewPlainSink(o)
	if !s.Quiet || !s.NoWarnings || !s.NoProgress {
		t.Fatalf("options not propagated: %+v", s)
	}
}
