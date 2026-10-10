package core

import (
	"reflect"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func TestParsePlaylistItems(t *testing.T) {
	cases := []struct {
		spec string
		want []int32
		ok   bool
	}{
		{"1-3", []int32{1, 2, 3}, true},
		{"1,3-4,9-", []int32{1, 3, 4, 9, 10}, true},
		{"-2", []int32{1, 2}, true},
		{"5", []int32{5}, true},
		{"1-3, 7", []int32{1, 2, 3, 7}, true},
		{"3-1", nil, false},
		{"abc", nil, false},
		{"1..3", nil, false},
		{"", []int32{}, true},
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
		var gotKeys []int32
		for k := range got {
			gotKeys = append(gotKeys, int32(k))
		}
		sortKeys(gotKeys)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(gotKeys, c.want) {
			t.Errorf("parsePlaylistItems(%q) = %v, want %v", c.spec, gotKeys, c.want)
		}
	}
}

func TestSelectPlaylistEntries(t *testing.T) {
	entries := make([]*infodict.InfoDict, 5)
	for i := range entries {
		entries[i] = &infodict.InfoDict{ID: runeID(i + 1)}
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
}

func runeID(n int) string { return string(rune('0' + n)) }

func sortKeys(keys []int32) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}
