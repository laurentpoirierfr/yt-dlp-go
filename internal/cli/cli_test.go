package cli

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func TestTUIAvailable(t *testing.T) {
	cases := []struct {
		name      string
		tui       bool
		supported bool
		hasTTY    bool
		want      bool
	}{
		{"auto on tty", false, true, false, true},
		{"auto off non-tty", false, false, false, false},
		{"forced via /dev/tty", true, false, true, true},
		{"forced but no terminal", true, false, false, false},
		{"forced and stdout tty", true, true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := &options.Options{TUI: c.tui}
			if got := tuiAvailable(o, c.supported, c.hasTTY); got != c.want {
				t.Fatalf("tuiAvailable() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestTUIEnabled(t *testing.T) {
	cases := []struct {
		name      string
		noTUI     bool
		quiet     bool
		simulate  bool
		available bool
		want      bool
	}{
		{"available", false, false, false, true, true},
		{"unavailable", false, false, false, false, false},
		{"no-tui overrides", true, false, false, true, false},
		{"quiet disables", false, true, false, true, false},
		{"simulate disables", false, false, true, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := &options.Options{NoTUI: c.noTUI, Quiet: c.quiet}
			if got := tuiEnabled(o, c.simulate, c.available); got != c.want {
				t.Fatalf("tuiEnabled() = %v, want %v", got, c.want)
			}
		})
	}
}
