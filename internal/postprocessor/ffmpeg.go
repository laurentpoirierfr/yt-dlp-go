// Package postprocessor implements the media post-processing stage.
package postprocessor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// FFmpegMerger merges separate video/audio streams into one container.
type FFmpegMerger struct {
	Location string
	sink     events.Sink
}

// NewFFmpegMerger builds a merger using the given ffmpeg binary ("" = PATH).
// sink may be nil (events are discarded).
func NewFFmpegMerger(location string, sink events.Sink) *FFmpegMerger {
	if sink == nil {
		sink = events.NopSink{}
	}
	return &FFmpegMerger{Location: location, sink: sink}
}

func (m *FFmpegMerger) bin() string {
	if m.Location != "" {
		return m.Location
	}
	return "ffmpeg"
}

// Available reports whether ffmpeg can be found.
func (m *FFmpegMerger) Available() bool {
	_, err := exec.LookPath(m.bin())
	return err == nil
}

// Merge concatenates the given input files into dest using stream copy.
func (m *FFmpegMerger) Merge(inputs []string, dest string, info *infodict.InfoDict) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no inputs to merge")
	}
	m.sink.PostProcess(events.PostProcess{Info: info, Name: "Merger", Status: events.StatusStarted})
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	for _, in := range inputs {
		args = append(args, "-i", in)
	}
	args = append(args, "-c", "copy")
	args = append(args, dest)
	cmd := exec.Command(m.bin(), args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		werr := fmt.Errorf("ffmpeg merge failed: %v: %s", err, strings.TrimSpace(stderr.String()))
		m.sink.PostProcess(events.PostProcess{Info: info, Name: "Merger", Status: events.StatusError, Err: werr})
		return werr
	}
	m.sink.PostProcess(events.PostProcess{Info: info, Name: "Merger", Status: events.StatusFinished})
	return nil
}
