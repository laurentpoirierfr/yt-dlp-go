// Package render provides event sinks. PlainSink reproduces the traditional
// yt-dlp command-line output on top of the events stream.
package render

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// PlainSink writes human-readable lines to Err (and Out), honouring the
// quiet/verbose/no-progress flags. It matches the historical CLI output.
type PlainSink struct {
	Out        io.Writer
	Err        io.Writer
	Quiet      bool
	NoWarnings bool
	Verbose    bool
	NoProgress bool
}

// NewPlainSink builds a PlainSink configured from options.
func NewPlainSink(o *options.Options) *PlainSink {
	s := &PlainSink{Out: os.Stdout, Err: os.Stderr}
	if o != nil {
		s.Quiet = o.Quiet
		s.NoWarnings = o.NoWarnings
		s.Verbose = o.Verbose
		s.NoProgress = o.NoProgress
	}
	return s
}

// Log implements events.Sink.
func (s *PlainSink) Log(l events.Log) {
	switch l.Level {
	case events.LevelDebug:
		if s.Verbose {
			fmt.Fprintf(s.Err, "[debug] %s\n", l.Text)
		}
	case events.LevelInfo:
		if !s.Quiet {
			fmt.Fprintln(s.Err, l.Text)
		}
	case events.LevelWarning:
		if !s.NoWarnings && !s.Quiet {
			fmt.Fprintf(s.Err, "WARNING: %s\n", l.Text)
		}
	case events.LevelError:
		fmt.Fprintf(s.Err, "ERROR: %s\n", l.Text)
	}
}

// Progress implements events.Sink.
func (s *PlainSink) Progress(p events.Progress) {
	if s.NoProgress || s.Quiet {
		return
	}
	switch p.Status {
	case events.StatusStarted:
		fmt.Fprintf(s.Err, "[download] Destination: %s\n", p.Filename)
	case events.StatusFinished:
		elapsed := p.Elapsed.Seconds()
		if elapsed <= 0 {
			elapsed = 0.001
		}
		fmt.Fprintf(s.Err, "[download] 100%% of %s in %s at %.2fKiB/s\n",
			utils.FormatBytes(p.Downloaded), utils.FormatClock(p.Elapsed),
			float64(p.Downloaded)/1024/elapsed)
	}
}

// PostProcess implements events.Sink.
func (s *PlainSink) PostProcess(p events.PostProcess) {
	if s.Quiet {
		return
	}
	switch p.Status {
	case events.StatusStarted:
		fmt.Fprintf(s.Err, "[%s] Starting\n", p.Name)
	case events.StatusFinished:
		fmt.Fprintf(s.Err, "[%s] Finished\n", p.Name)
	case events.StatusError:
		fmt.Fprintf(s.Err, "[%s] Error: %v\n", p.Name, p.Err)
	}
}

// Ensure PlainSink implements the interface.
var _ events.Sink = (*PlainSink)(nil)

// FormatSpeed renders a byte-per-second rate.
func FormatSpeed(bps float64) string {
	return utils.FormatBytes(int64(bps)) + "/s"
}

// FormatETA renders an ETA as MM:SS.
func FormatETA(eta time.Duration) string {
	return utils.FormatClock(eta)
}
