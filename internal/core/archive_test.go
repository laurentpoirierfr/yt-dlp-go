package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func TestPlaylistMaxEntries(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*options.Options)
		want int
	}{
		{"default", func(o *options.Options) {}, 0},
		{"end", func(o *options.Options) { o.PlaylistEnd = 10 }, 10},
		{"bounded items", func(o *options.Options) { o.PlaylistItems = "1:3,7" }, 7},
		{"single item", func(o *options.Options) { o.PlaylistItems = "5" }, 5},
		{"negative item", func(o *options.Options) { o.PlaylistItems = "-1" }, 0},
		{"unbounded item", func(o *options.Options) { o.PlaylistItems = "7:" }, 0},
		{"negative step", func(o *options.Options) { o.PlaylistItems = "5:1:-1" }, 0},
		{"reverse", func(o *options.Options) { o.PlaylistReverse = true }, 0},
		{"random", func(o *options.Options) { o.PlaylistRandom = true }, 0},
		{"bad items", func(o *options.Options) { o.PlaylistItems = "abc" }, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := options.DefaultOptions()
			c.mut(o)
			ydl := New(o)
			if got := ydl.playlistMaxEntries(); got != c.want {
				t.Errorf("playlistMaxEntries() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestReverseAndShuffleItems(t *testing.T) {
	items := []playlistItem{
		{idx: 1, entry: &infodict.InfoDict{ID: "a"}},
		{idx: 2, entry: &infodict.InfoDict{ID: "b"}},
		{idx: 3, entry: &infodict.InfoDict{ID: "c"}},
	}
	got := make([]playlistItem, len(items))
	copy(got, items)
	reverseItems(got)
	if got[0].idx != 3 || got[2].idx != 1 || got[1].idx != 2 {
		t.Errorf("reverseItems = %v, want [3 2 1]", indices(got))
	}
	shuffleItems(got)
	if len(got) != 3 {
		t.Fatalf("shuffleItems changed size")
	}
	seen := map[int]bool{}
	for _, it := range got {
		if seen[it.idx] {
			t.Fatalf("shuffleItems duplicated an item: %v", indices(got))
		}
		seen[it.idx] = true
	}
}

func TestArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.txt")
	o := options.DefaultOptions()
	o.DownloadArchive = path
	ydl := New(o)

	info := &infodict.InfoDict{ID: "dQw4w9WgXcQ", Extractor: "Youtube", ExtractorKey: "youtube"}
	if ydl.inArchive(info) {
		t.Fatal("inArchive true before adding")
	}
	if err := ydl.addArchive(info); err != nil {
		t.Fatalf("addArchive: %v", err)
	}
	if !ydl.inArchive(info) {
		t.Fatal("inArchive false after adding")
	}
	if err := ydl.addArchive(info); err != nil {
		t.Fatalf("addArchive again: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "youtube dQw4w9WgXcQ\n" {
		t.Errorf("archive content = %q, want single line", string(b))
	}

	// pre-existing entries are picked up on load
	if err := os.WriteFile(path, []byte("generic abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ydl2 := New(o)
	if !ydl2.inArchive(&infodict.InfoDict{ID: "abc"}) {
		t.Error("pre-existing archive entry not detected")
	}
}

func TestArchiveKey(t *testing.T) {
	if got := archiveKey(&infodict.InfoDict{ID: "x", ExtractorKey: "youtube"}); got != "youtube x" {
		t.Errorf("archiveKey = %q", got)
	}
	if got := archiveKey(&infodict.InfoDict{ID: "x", Extractor: "Generic"}); got != "generic x" {
		t.Errorf("archiveKey empty key = %q", got)
	}
	if got := archiveKey(&infodict.InfoDict{ID: "x"}); got != "generic x" {
		t.Errorf("archiveKey fallback = %q", got)
	}
}
