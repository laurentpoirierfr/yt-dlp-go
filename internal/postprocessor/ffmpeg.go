// Package postprocessor implements the media post-processing stage.
package postprocessor

import (
	"fmt"
	"os/exec"
	"strings"
)

// FFmpegMerger merges separate video/audio streams into one container.
type FFmpegMerger struct {
	Location string
}

// NewFFmpegMerger builds a merger using the given ffmpeg binary ("" = PATH).
func NewFFmpegMerger(location string) *FFmpegMerger {
	return &FFmpegMerger{Location: location}
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
func (m *FFmpegMerger) Merge(inputs []string, dest string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no inputs to merge")
	}
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
		return fmt.Errorf("ffmpeg merge failed: %v: %s", err, stderr.String())
	}
	return nil
}
