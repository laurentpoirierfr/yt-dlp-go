package cli

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func TestValidateUnsupportedRejects(t *testing.T) {
	cases := [][]string{
		{"--write-subs", "URL"},
		{"--write-thumbnail", "URL"},
		{"--limit-rate", "1000", "URL"},
		{"--remux-video", "mkv", "URL"},
	}
	for _, args := range cases {
		cmd := newRootCmd(options.DefaultOptions())
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("Execute(%v) = nil, want unsupported-option error", args)
		}
	}
}

func TestValidateUnsupportedAcceptsImplemented(t *testing.T) {
	cases := [][]string{
		{"--restrict-filenames", "--ignore-errors", "--force-overwrites", "URL"},
		{"--playlist-reverse", "--playlist-random", "URL"},
		{"--break-on-existing", "--break-per-input", "--download-archive", "archive.txt", "URL"},
		{"--write-info-json", "--force-overwrites", "URL"},
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
