package core

import (
	"reflect"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func TestParsePlaylistItems(t *testing.T) {
	cases := []struct {
		spec string
		want []playlistRange
		ok   bool
	}{
		{"1-3", []playlistRange{{1, 3}}, true},
		{"1,3-4,9-", []playlistRange{{1, 1}, {3, 4}, {9, 0}}, true},
		{"-2", []playlistRange{{1, 2}}, true},
		{"5", []playlistRange{{5, 5}}, true},
		{"1-3, 7", []playlistRange{{1, 3}, {7, 7}}, true},
		{"", nil, true},
		{"3-1", nil, false},
		{"abc", nil, false},
		{"1..3", nil, false},
	}
	for _, c := range cases {
		got, err := parsePlaylistItems(c.spec)
		if c.ok != (err == nil) {
			t.Errorf("parsePlaylistItems(%q) err = %v, want ok=%v", c.spec, err, c.ok)
			continue
		}
		if !c.ok {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parsePlaylistItems(%q) = %v, want %v", c.spec, got, c.want)
		}
	}
}

func TestSelectPlaylistEntries(t *testing.T) {
	entries := make([]*infodict.InfoDict, 5)
	for i := range entries {
		entries[i] = &infodict.InfoDict{ID: string(rune('0' + i + 1))}
	}
	selected, err := selectPlaylistEntries(entries, 2, 4, "")
	if err != nil {
		t.Fatalf("selection err: %v", err)
	}
	wantIdx := []int{2, 3, 4}
	if len(selected) != 3 {
		t.Fatalf("len = %d, want 3", len(selected))
	}
	for i, it := range selected {
		if it.idx != wantIdx[i] {
			t.Errorf("idx = %d, want %d", it.idx, wantIdx[i])
		}
	}

	selected, err = selectPlaylistEntries(entries, 1, 0, "1,4")
	if err != nil {
		t.Fatalf("items err: %v", err)
	}
	if len(selected) != 2 || selected[0].idx != 1 || selected[1].idx != 4 {
		t.Errorf("items selection = %+v", selected)
	}

	selected, err = selectPlaylistEntries(entries, 1, 0, "4-")
	if err != nil {
		t.Fatalf("unbounded err: %v", err)
	}
	if len(selected) != 2 || selected[0].idx != 4 || selected[1].idx != 5 {
		t.Errorf("unbounded selection = %+v", selected)
	}

	selected, err = selectPlaylistEntries(entries, 1, 0, "9-")
	if err != nil {
		t.Fatalf("out-of-range unbounded err: %v", err)
	}
	if len(selected) != 0 {
		t.Errorf("out-of-range selection = %+v, want none", selected)
	}
}
