package core

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func ydlFor(tmpl string) *YoutubeDL {
	o := options.DefaultOptions()
	o.Outtmpl["default"] = tmpl
	return New(o)
}

func TestBuildOutputPathSanitizesValues(t *testing.T) {
	y := ydlFor("%(title)s [%(id)s].%(ext)s")
	info := &infodict.InfoDict{ID: "abc123", Title: "My: Video/Part?", Ext: "mp4"}

	got, err := y.buildOutputPath(info)
	if err != nil {
		t.Fatalf("buildOutputPath: %v", err)
	}
	want := "My_ Video_Part_ [abc123].mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathNoSeparatorInjection(t *testing.T) {
	y := ydlFor("%(uploader)s/%(title)s.%(ext)s")
	info := &infodict.InfoDict{Uploader: "a/b", Title: "c", Ext: "mp4"}

	got, err := y.buildOutputPath(info)
	if err != nil {
		t.Fatalf("buildOutputPath: %v", err)
	}
	want := "a_b/c.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathRestrictFilenames(t *testing.T) {
	y := ydlFor("%(title)s.%(ext)s")
	y.opts.RestrictFilenames = true
	info := &infodict.InfoDict{Title: "Café délicieux", Ext: "mp4"}

	got, _ := y.buildOutputPath(info)
	want := "Caf__d_licieux.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathWindowsFilenames(t *testing.T) {
	y := ydlFor("%(title)s.%(ext)s")
	y.opts.WindowsFilenames = true
	info := &infodict.InfoDict{Title: "Hello. ", Ext: "mp4"}

	got, _ := y.buildOutputPath(info)
	want := "Hello.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathTrimFilenames(t *testing.T) {
	y := ydlFor("%(title)s.%(ext)s")
	y.opts.TrimFileName = 5
	info := &infodict.InfoDict{Title: "ABCDEFG", Ext: "mp4"}

	got, _ := y.buildOutputPath(info)
	want := "ABCDE"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathFormatSpecifier(t *testing.T) {
	y := ydlFor("%(playlist_index)02d - %(title)s.%(ext)s")
	idx := 7
	info := &infodict.InfoDict{Title: "X", Ext: "mp4", PlaylistIndex: &idx}

	got, _ := y.buildOutputPath(info)
	want := "07 - X.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathDefaultAlternative(t *testing.T) {
	y := ydlFor("%(uploader|unknown)s/%(title)s.%(ext)s")
	info := &infodict.InfoDict{Title: "X", Ext: "mp4"}

	got, _ := y.buildOutputPath(info)
	want := "unknown/X.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathNAPlaceholder(t *testing.T) {
	y := ydlFor("%(height)s.%(ext)s")
	info := &infodict.InfoDict{Ext: "mp4"}

	got, _ := y.buildOutputPath(info)
	want := "NA.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOutputPathCustomNAPlaceholder(t *testing.T) {
	y := ydlFor("%(height)s.%(ext)s")
	y.opts.OuttmplNAPlaceholder = "none"
	info := &infodict.InfoDict{Ext: "mp4"}

	got, _ := y.buildOutputPath(info)
	want := "none.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyVerb(t *testing.T) {
	cases := []struct {
		val, verb, want string
	}{
		{"7", "02d", "07"},
		{"7", "d", "7"},
		{"3.14159", ".2f", "3.14"},
		{"hello", "s", "hello"},
		{"hello", "10s", "     hello"},
		{"", "02d", ""},
		{"NA", "02d", "NA"},
	}
	for _, c := range cases {
		if got := applyVerb(c.val, c.verb); got != c.want {
			t.Errorf("applyVerb(%q, %q) = %q, want %q", c.val, c.verb, got, c.want)
		}
	}
}
