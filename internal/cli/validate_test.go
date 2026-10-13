package cli

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func TestValidateUnsupportedRejects(t *testing.T) {
	cases := [][]string{
		{"--limit-rate", "1000", "URL"},
		{"--remux-video", "mkv", "URL"},
		{"--convert-subs", "ass", "URL"},
	}
	for _, args := range cases {
		cmd := newRootCmd(options.DefaultOptions())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("Execute(%v) = nil, want unsupported-option error", args)
		}
	}
}

func TestValidateMergeOutputFormatRejects(t *testing.T) {
	cases := [][]string{
		{"--merge-output-format", "url/avi", "URL"},
		{"--merge-output-format", "MP4", "URL"},
		{"--merge-output-format", "", "URL"},
	}
	for _, args := range cases {
		cmd := newRootCmd(options.DefaultOptions())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("Execute(%v) = nil, want invalid merge output format error", args)
		}
	}
}

func TestValidateMergeOutputFormatAccepts(t *testing.T) {
	cases := [][]string{
		{"--merge-output-format", "mp4/mkv", "URL"},
		{"--merge-output-format", "mp4/avi", "URL"},
		{"--merge-output-format", "avimkv", "URL"},
	}
	for _, args := range cases {
		cmd := newRootCmd(options.DefaultOptions())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("Execute(%v) = %v, want nil", args, err)
		}
	}
}

func TestValidateUnsupportedAcceptsImplemented(t *testing.T) {
	cases := [][]string{
		{"--restrict-filenames", "--ignore-errors", "--force-overwrites", "URL"},
		{"--playlist-reverse", "--playlist-random", "URL"},
		{"--break-on-existing", "--break-per-input", "--download-archive", "archive.txt", "URL"},
		{"--write-info-json", "--force-overwrites", "URL"},
		{"--write-subs", "--write-auto-subs", "--all-subs", "URL"},
		{"--sub-langs", "en,fr", "--sub-format", "vtt", "--list-subs", "URL"},
		{"--convert-subs", "srt", "URL"},
		{"--write-thumbnail", "--write-all-thumbnails", "--list-thumbnails", "URL"},
		{"--write-description", "URL"},
	}
	for _, args := range cases {
		cmd := newRootCmd(options.DefaultOptions())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("Execute(%v) = %v, want nil", args, err)
		}
	}
}

func TestIgnoreErrorsDefaultsAndFlag(t *testing.T) {
	if got := options.DefaultOptions().IgnoreErrors; got != options.IgnoreNone {
		t.Errorf("default IgnoreErrors = %v, want IgnoreNone", got)
	}
	o := options.DefaultOptions()
	cmd := newRootCmd(o)
	cmd.SetArgs([]string{"-i", "URL"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if o.IgnoreErrors != options.IgnoreAll {
		t.Errorf("-i set IgnoreErrors = %v, want IgnoreAll", o.IgnoreErrors)
	}
}
