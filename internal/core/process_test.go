package core

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func mkVideo(id string) *infodict.InfoDict {
	return &infodict.InfoDict{
		ID: id, Type: "video", Title: id, Extractor: "test", ExtractorKey: "test",
		Formats: []infodict.Format{{URL: "http://x/" + id, Ext: "mp4", FormatID: "1", VCodec: "avc1", ACodec: "mp4a"}},
	}
}

func mkPlaylist(ids ...string) *infodict.InfoDict {
	entries := make([]*infodict.InfoDict, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, mkVideo(id))
	}
	return &infodict.InfoDict{Type: "playlist", ID: "PL1", Title: "p", Entries: entries}
}

// processPlaylistOrder runs processPlaylist with a downloader that records the
// processed entry ids and returns them.
func processPlaylistOrder(t *testing.T, o *options.Options, pl *infodict.InfoDict) []string {
	t.Helper()
	ydl := New(o)
	var order []string
	ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error {
		order = append(order, info.ID)
		return nil
	})
	if _, err := ydl.processPlaylist(pl, true, nil, 0); err != nil {
		t.Fatalf("processPlaylist: %v", err)
	}
	return order
}

func TestProcessPlaylistOrdering(t *testing.T) {
	base := func() *options.Options {
		o := options.DefaultOptions()
		o.PlaylistStart = 1
		return o
	}
	cases := []struct {
		name string
		mut  func(*options.Options)
		want []string
	}{
		{"start-end window", func(o *options.Options) { o.PlaylistEnd = 3 }, []string{"a", "b", "c"}},
		{"reverse window", func(o *options.Options) { o.PlaylistEnd = 3; o.PlaylistReverse = true }, []string{"c", "b", "a"}},
		{"reverse unbounded", func(o *options.Options) { o.PlaylistReverse = true }, []string{"e", "d", "c", "b", "a"}},
		{"items", func(o *options.Options) { o.PlaylistItems = "2:4" }, []string{"b", "c", "d"}},
		{"items reverse", func(o *options.Options) { o.PlaylistItems = "1:4"; o.PlaylistReverse = true }, []string{"d", "c", "b", "a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := base()
			c.mut(o)
			got := processPlaylistOrder(t, o, mkPlaylist("a", "b", "c", "d", "e"))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("order = %v, want %v", got, c.want)
			}
		})
	}
}

func TestProcessPlaylistRandom(t *testing.T) {
	o := options.DefaultOptions()
	o.PlaylistStart = 1
	o.PlaylistRandom = true
	got := processPlaylistOrder(t, o, mkPlaylist("a", "b", "c", "d", "e"))
	if len(got) != 5 {
		t.Fatalf("got %d ids, want 5", len(got))
	}
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Errorf("duplicate id %q in %v", id, got)
		}
		seen[id] = true
	}
}

func TestProcessPlaylistBreakOnExisting(t *testing.T) {
	o := options.DefaultOptions()
	o.PlaylistStart = 1
	o.DownloadArchive = filepath.Join(t.TempDir(), "arch.txt")
	o.BreakOnExisting = true
	ydl := New(o)
	if err := ydl.addArchive(mkVideo("a")); err != nil {
		t.Fatal(err)
	}
	var order []string
	ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error {
		order = append(order, info.ID)
		return nil
	})
	if _, err := ydl.processPlaylist(mkPlaylist("a", "b", "c"), true, nil, 0); err != nil {
		t.Fatalf("processPlaylist: %v", err)
	}
	if len(order) != 0 {
		t.Errorf("expected to stop at archived entry, downloaded %v", order)
	}
}

func TestProcessPlaylistRecordsArchive(t *testing.T) {
	o := options.DefaultOptions()
	o.PlaylistStart = 1
	o.DownloadArchive = filepath.Join(t.TempDir(), "arch.txt")
	processPlaylistOrder(t, o, mkPlaylist("x", "y"))
	ydl := New(o)
	if !ydl.inArchive(mkVideo("x")) || !ydl.inArchive(mkVideo("y")) {
		t.Error("downloaded entries were not recorded in the archive")
	}
	if ydl.inArchive(mkVideo("z")) {
		t.Error("untouched entry should not be in archive")
	}
}

func TestProcessPlaylistBreakPerInput(t *testing.T) {
	bad := mkVideo("bad")
	bad.Formats = nil // provokes a NoFormatsError during processing
	pl := &infodict.InfoDict{Type: "playlist", ID: "PL1", Title: "p",
		Entries: []*infodict.InfoDict{mkVideo("a"), bad, mkVideo("c")}}
	o := options.DefaultOptions()
	o.PlaylistStart = 1
	o.BreakPerURL = true
	ydl := New(o)
	var order []string
	ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error {
		order = append(order, info.ID)
		return nil
	})
	if _, err := ydl.processPlaylist(pl, true, nil, 0); err != nil {
		t.Fatalf("processPlaylist: %v", err)
	}
	if got := order; !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("order = %v, want [a] (stop after first error)", got)
	}
}
