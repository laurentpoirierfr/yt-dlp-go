package cli

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func TestTUIEnabled(t *testing.T) {
	cases := []struct {
		name     string
		tui      bool
		noTUI    bool
		quiet    bool
		simulate bool
		tty      bool
		want     bool
	}{
		{"auto on tty", false, false, false, false, true, true},
		{"auto off non-tty", false, false, false, false, false, false},
		{"forced on non-tty", true, false, false, false, false, true},
		{"no-tui overrides force", true, true, false, false, true, false},
		{"quiet disables", false, false, true, false, true, false},
		{"simulate disables", false, false, false, true, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := &options.Options{TUI: c.tui, NoTUI: c.noTUI, Quiet: c.quiet}
			if got := tuiEnabled(o, c.simulate, c.tty); got != c.want {
				t.Fatalf("tuiEnabled() = %v, want %v", got, c.want)
			}
		})
	}
}
