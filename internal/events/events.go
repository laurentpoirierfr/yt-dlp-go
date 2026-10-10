// Package events defines the decoupled notification stream emitted by the
// extraction/download pipeline. It is the Go equivalent of yt-dlp's
// progress_hooks / postprocessor_hooks and lets any front-end (plain CLI, TUI,
// library caller) observe progress without the core writing to stdio.
package events

import (
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// Status is the lifecycle state of a job or postprocessor.
type Status int

const (
	// StatusStarted is emitted once before work begins (file opened / PP queued).
	StatusStarted Status = iota
	// StatusRunning is emitted periodically while work is in progress.
	StatusRunning
	// StatusFinished is emitted once when work completed successfully.
	StatusFinished
	// StatusError is emitted when work failed (Err is set).
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusStarted:
		return "started"
	case StatusRunning:
		return "running"
	case StatusFinished:
		return "finished"
	case StatusError:
		return "error"
	}
	return "unknown"
}

// Level is a log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarning
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarning:
		return "warning"
	case LevelError:
		return "error"
	}
	return "unknown"
}

// Progress reports the state of a single download.
type Progress struct {
	Info     *infodict.InfoDict
	Filename string
	Status   Status

	Downloaded int64         // bytes written so far (including resumed part)
	Total      int64         // total bytes, -1 if unknown
	Speed      float64       // bytes per second
	ETA        time.Duration // estimated time remaining, <0 if unknown
	Elapsed    time.Duration

	FragmentIndex int // 1-based, 0 when not fragmented
	FragmentCount int // 0 when not fragmented

	Err error
}

// PostProcess reports the state of a post-processing step (merge, remux, …).
type PostProcess struct {
	Info   *infodict.InfoDict
	Name   string // "Merger", "ExtractAudio", …
	Status Status
	Err    error
}

// Log is a free-form message at a given severity.
type Log struct {
	Level Level
	Text  string
}

// Sink receives pipeline events. Implementations must be safe to call from
// arbitrary goroutines if downloads run concurrently.
type Sink interface {
	Progress(Progress)
	PostProcess(PostProcess)
	Log(Log)
}

// FuncSink adapts a set of functions to the Sink interface; nil funcs are
// ignored.
type FuncSink struct {
	ProgressFunc    func(Progress)
	PostProcessFunc func(PostProcess)
	LogFunc         func(Log)
}

func (f FuncSink) Progress(p Progress) {
	if f.ProgressFunc != nil {
		f.ProgressFunc(p)
	}
}

func (f FuncSink) PostProcess(p PostProcess) {
	if f.PostProcessFunc != nil {
		f.PostProcessFunc(p)
	}
}

func (f FuncSink) Log(l Log) {
	if f.LogFunc != nil {
		f.LogFunc(l)
	}
}

// NopSink discards every event.
type NopSink struct{}

func (NopSink) Progress(Progress)       {}
func (NopSink) PostProcess(PostProcess) {}
func (NopSink) Log(Log)                 {}

// MultiSink fans out to several sinks in order.
type MultiSink []Sink

func (m MultiSink) Progress(p Progress) {
	for _, s := range m {
		s.Progress(p)
	}
}

func (m MultiSink) PostProcess(p PostProcess) {
	for _, s := range m {
		s.PostProcess(p)
	}
}

func (m MultiSink) Log(l Log) {
	for _, s := range m {
		s.Log(l)
	}
}
