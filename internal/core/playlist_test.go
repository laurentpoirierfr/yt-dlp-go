package core

import (
	"reflect"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func TestParsePlaylistItems(t *testing.T) {
	cases := []struct {
		spec string
		want []itemRange
		ok   bool
	}{
		{"", nil, true},
		{"5", []itemRange{{start: 5, end: 5, step: 1}}, true},
		{"1-3", []itemRange{{start: 1, end: 3, step: 1}}, true},
		{"1,3-4,9-", []itemRange{
			{start: 1, end: 1, step: 1},
			{start: 3, end: 4, step: 1},
			{start: 9, end: 0, step: 1, endUnbounded: true},
		}, true},
		{"-2", []itemRange{{start: -2, end: -2, step: 1}}, true},
		{"1:10", []itemRange{{start: 1, end: 10, step: 1}}, true},
		{":3", []itemRange{{start: 1, end: 3, step: 1}}, true},
		{"7:", []itemRange{{start: 7, end: 0, step: 1, endUnbounded: true}}, true},
		{"1:10:2", []itemRange{{start: 1, end: 10, step: 2}}, true},
		{"5:1:-1", []itemRange{{start: 5, end: 1, step: -1}}, true},
		{"1:3,7,-5::2", []itemRange{
			{start: 1, end: 3, step: 1},
			{start: 7, end: 7, step: 1},
			{start: -5, end: 0, step: 2, endUnbounded: true},
		}, true},
		{"3-1", nil, false},
		{"abc", nil, false},
		{"1..3", nil, false},
		{"1:2:0", nil, false},
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

func indices(items []playlistItem) []int {
	if len(items) == 0 {
		return nil
	}
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = it.idx
	}
	return out
}

func TestSelectPlaylistEntries(t *testing.T) {
	entries := make([]*infodict.InfoDict, 5)
	for i := range entries {
		entries[i] = &infodict.InfoDict{ID: string(rune('0' + i + 1))}
	}
	cases := []struct {
		start, end int
		items      string
		want       []int
	}{
		{2, 4, "", []int{2, 3, 4}},
		{1, 0, "1,4", []int{1, 4}},
		{1, 0, "4-", []int{4, 5}},
		{1, 0, "9-", nil},
		{1, 0, "-1", []int{5}},
		{1, 0, "-2", []int{4}},
		{1, 0, "1:10", []int{1, 2, 3, 4, 5}},
		{1, 0, "1,3,5,-1", []int{1, 3, 5}},
		{1, 0, "5:1:-1", []int{5, 4, 3, 2, 1}},
		{1, 0, "2,4,1", []int{2, 4, 1}},
	}
	for _, c := range cases {
		selected, err := selectPlaylistEntries(entries, c.start, c.end, c.items)
		if err != nil {
			t.Errorf("selectPlaylistEntries(items=%q): %v", c.items, err)
			continue
		}
		if got := indices(selected); !reflect.DeepEqual(got, c.want) {
			t.Errorf("selectPlaylistEntries(start=%d,end=%d,items=%q) = %v, want %v",
				c.start, c.end, c.items, got, c.want)
		}
	}
}
