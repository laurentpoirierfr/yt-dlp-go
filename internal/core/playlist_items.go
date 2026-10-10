package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// itemRange is a parsed --playlist-items entry. Bounds are 1-based; negative
// values count from the end of the playlist. endUnbounded extends the range to
// the end of the playlist, and a negative step selects in reverse order.
type itemRange struct {
	start        int
	end          int
	step         int
	endUnbounded bool
}

// selectPlaylistEntries narrows the entries to those selected by
// --playlist-start, --playlist-end and --playlist-items.
func selectPlaylistEntries(entries []*infodict.InfoDict, start, end int, items string) ([]playlistItem, error) {
	n := len(entries)
	if items != "" {
		ranges, err := parsePlaylistItems(items)
		if err != nil {
			return nil, err
		}
		var out []playlistItem
		seen := make(map[int]bool)
		for _, r := range ranges {
			for _, idx := range r.resolve(n) {
				if seen[idx] {
					continue
				}
				entry := entries[idx-1]
				if entry == nil {
					continue
				}
				seen[idx] = true
				out = append(out, playlistItem{idx: idx, entry: entry})
			}
		}
		return out, nil
	}
	var out []playlistItem
	for i, entry := range entries {
		if entry == nil {
			continue
		}
		idx := i + 1
		if idx < start || (end > 0 && idx > end) {
			continue
		}
		out = append(out, playlistItem{idx: idx, entry: entry})
	}
	return out, nil
}

// parsePlaylistItems parses a yt-dlp compatible --playlist-items specification
// such as "1:3,7,-5::2" or the legacy "1-3,5,9-".
func parsePlaylistItems(spec string) ([]itemRange, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var out []itemRange
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		r, err := parseItemRange(part)
		if err != nil {
			return nil, fmt.Errorf("invalid format for --playlist-items: %q", part)
		}
		out = append(out, r)
	}
	return out, nil
}

func parseItemRange(tok string) (itemRange, error) {
	if strings.Contains(tok, ":") {
		return parseColonRange(tok)
	}
	// Legacy dash range: "START-STOP" with a positive START. A leading dash
	// denotes a negative (from-the-end) index instead.
	if dash := strings.IndexByte(tok, '-'); dash > 0 {
		start, err := strconv.Atoi(strings.TrimSpace(tok[:dash]))
		if err != nil {
			return itemRange{}, err
		}
		r := itemRange{start: start, step: 1}
		right := strings.TrimSpace(tok[dash+1:])
		if right == "" {
			r.endUnbounded = true
			return r, nil
		}
		if r.end, err = strconv.Atoi(right); err != nil {
			return itemRange{}, err
		}
		if r.start > r.end {
			return itemRange{}, fmt.Errorf("start %d is greater than end %d", r.start, r.end)
		}
		return r, nil
	}
	n, err := strconv.Atoi(tok)
	if err != nil {
		return itemRange{}, err
	}
	return itemRange{start: n, end: n, step: 1}, nil
}

func parseColonRange(tok string) (itemRange, error) {
	fields := strings.Split(tok, ":")
	if len(fields) > 3 {
		return itemRange{}, fmt.Errorf("too many ':' separators")
	}
	r := itemRange{step: 1}
	var err error
	if fields[0] == "" {
		r.start = 1
	} else if r.start, err = strconv.Atoi(strings.TrimSpace(fields[0])); err != nil {
		return itemRange{}, err
	}
	if len(fields) >= 2 {
		if strings.TrimSpace(fields[1]) == "" {
			r.endUnbounded = true
		} else if r.end, err = strconv.Atoi(strings.TrimSpace(fields[1])); err != nil {
			return itemRange{}, err
		}
	} else {
		r.end = r.start
	}
	if len(fields) == 3 {
		if strings.TrimSpace(fields[2]) == "" {
			return itemRange{}, fmt.Errorf("empty step")
		}
		if r.step, err = strconv.Atoi(strings.TrimSpace(fields[2])); err != nil {
			return itemRange{}, err
		}
		if r.step == 0 {
			return itemRange{}, fmt.Errorf("step must not be zero")
		}
	}
	return r, nil
}

// resolve returns the concrete 1-based indices selected by r within a playlist
// of n entries, in the order implied by the range's step.
func (r itemRange) resolve(n int) []int {
	abs := func(v int) int {
		if v < 0 {
			return n + 1 + v
		}
		return v
	}
	start := abs(r.start)
	end := r.end
	if r.endUnbounded {
		if r.step < 0 {
			end = 1
		} else {
			end = n
		}
	} else {
		end = abs(r.end)
	}
	if r.step == 0 {
		return nil
	}
	var out []int
	if r.step > 0 {
		for k := start; k <= end; k += r.step {
			if k >= 1 && k <= n {
				out = append(out, k)
			}
		}
	} else {
		for k := start; k >= end; k += r.step {
			if k >= 1 && k <= n {
				out = append(out, k)
			}
		}
	}
	return out
}
