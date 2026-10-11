// Package subtitles implements subtitle language/format selection and
// conversion between the formats yt-dlp can write (WebVTT, SRT, json3).
package subtitles

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// preference is the ranking used for --sub-format best, from lowest to highest
// preference (mirrors the order YouTube exposes caption tracks in).
var preference = []string{"srv1", "srv2", "srv3", "json3", "ttml", "vtt"}

func rank(ext string) int {
	for i, e := range preference {
		if e == ext {
			return i
		}
	}
	return -1
}

// SelectLangs returns the available subtitle languages matching the requested
// specifications, sorted. An empty spec, or one containing "all", selects every
// language. Entries are treated as regular expressions; a "-" prefix excludes
// matching languages.
func SelectLangs(available map[string][]infodict.Subtitle, spec []string) []string {
	all := len(spec) == 0
	var includes, excludes []*regexp.Regexp
	var includeLiterals, excludeLiterals []string
	add := func(dst *[]*regexp.Regexp, lits *[]string, pat string) {
		if re, err := regexp.Compile("^(?:" + pat + ")$"); err == nil {
			*dst = append(*dst, re)
		} else {
			*lits = append(*lits, pat)
		}
	}
	for _, s := range spec {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if s == "all" {
			all = true
			continue
		}
		if strings.HasPrefix(s, "-") {
			add(&excludes, &excludeLiterals, strings.TrimPrefix(s, "-"))
			continue
		}
		add(&includes, &includeLiterals, s)
	}
	matches := func(res []*regexp.Regexp, lits []string, lang string) bool {
		for _, re := range res {
			if re.MatchString(lang) {
				return true
			}
		}
		for _, l := range lits {
			if l == lang {
				return true
			}
		}
		return false
	}
	var out []string
	for lang := range available {
		switch {
		case matches(excludes, excludeLiterals, lang):
			continue
		case all || matches(includes, includeLiterals, lang):
			out = append(out, lang)
		}
	}
	sort.Strings(out)
	return out
}

// Choose picks the best subtitle representation for the given --sub-format
// preference (a "/"-separated list, e.g. "ass/srt/best"; empty means "best").
// For "best" the highest-preference available format is returned.
func Choose(subs []infodict.Subtitle, format string) (infodict.Subtitle, bool) {
	if format == "" {
		format = "best"
	}
	prefs := strings.Split(format, "/")
	for _, p := range prefs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "best" {
			best := infodict.Subtitle{}
			bestRank := -1
			for _, s := range subs {
				if r := rank(s.Ext); r > bestRank {
					best, bestRank = s, r
				}
			}
			if bestRank >= 0 {
				return best, true
			}
			continue
		}
		for _, s := range subs {
			if strings.EqualFold(s.Ext, p) {
				return s, true
			}
		}
	}
	return infodict.Subtitle{}, false
}

// Convert rewrites subtitle data from fromExt to toExt, returning the new data
// and extension. When no conversion is needed the input is returned unchanged.
func Convert(data []byte, fromExt, toExt string) ([]byte, string, error) {
	if toExt == "" || strings.EqualFold(fromExt, toExt) {
		return data, fromExt, nil
	}
	switch toExt {
	case "srt":
		switch fromExt {
		case "vtt":
			return []byte(VTTToSRT(string(data))), "srt", nil
		case "json3":
			return []byte(JSON3ToSRT(data)), "srt", nil
		}
	case "vtt":
		if fromExt == "json3" {
			return []byte(JSON3ToVTT(data)), "vtt", nil
		}
	}
	return nil, "", fmt.Errorf("cannot convert subtitles from %q to %q", fromExt, toExt)
}
