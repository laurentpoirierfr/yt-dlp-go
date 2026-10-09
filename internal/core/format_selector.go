package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/elfeo/yt-dlp-go/internal/infodict"
)

var audioExts = map[string]bool{
	"aac": true, "ape": true, "asf": true, "f4a": true, "f4b": true, "m4b": true,
	"m4r": true, "oga": true, "ogx": true, "spx": true, "vorbis": true, "wma": true,
	"weba": true, "aiff": true, "alac": true, "flac": true, "m4a": true, "mka": true,
	"mp3": true, "ogg": true, "opus": true, "wav": true,
}

var videoExts = map[string]bool{
	"3g2": true, "3gp": true, "f4v": true, "mk3d": true, "divx": true, "mpg": true,
	"ogv": true, "m4v": true, "wmv": true, "avi": true, "flv": true, "mkv": true,
	"mov": true, "mp4": true, "webm": true,
}

var storyboardExts = map[string]bool{"mhtml": true}

// selectorFn picks formats from a slice.
type selectorFn func(formats []infodict.Format) []infodict.Format

func hasVideo(f infodict.Format) bool { return f.VCodec != "" && f.VCodec != "none" }
func hasAudio(f infodict.Format) bool { return f.ACodec != "" && f.ACodec != "none" }

// selectFormats applies a format specification to the given formats.
func (y *YoutubeDL) selectFormats(formats []infodict.Format, spec string) ([]infodict.Format, error) {
	fn, err := y.buildFormatSelector(spec)
	if err != nil {
		return nil, err
	}
	return fn(formats), nil
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

type selNode struct {
	typ     string // single, pickfirst, merge, group
	spec    string
	filters []string
	a, b    *selNode
	list    []*selNode
}

type fmtScanner struct {
	s   string
	pos int
}

func (sc *fmtScanner) next() (typ, val string, ok bool) {
	for sc.pos < len(sc.s) && (sc.s[sc.pos] == ' ') {
		sc.pos++
	}
	if sc.pos >= len(sc.s) {
		return "", "", false
	}
	c := sc.s[sc.pos]
	switch c {
	case '/', '+', ',', '(', ')':
		sc.pos++
		return "op", string(c), true
	case '[':
		sc.pos++
		start := sc.pos
		for sc.pos < len(sc.s) && sc.s[sc.pos] != ']' {
			sc.pos++
		}
		val := sc.s[start:sc.pos]
		if sc.pos < len(sc.s) {
			sc.pos++
		}
		return "filter", val, true
	}
	start := sc.pos
	for sc.pos < len(sc.s) {
		ch := sc.s[sc.pos]
		if strings.ContainsRune("/+,()[] ", rune(ch)) {
			break
		}
		sc.pos++
	}
	return "name", sc.s[start:sc.pos], true
}

func (y *YoutubeDL) buildFormatSelector(spec string) (selectorFn, error) {
	sc := &fmtScanner{s: spec}
	sel, _, err := parseSelection(sc, false, false, false)
	if err != nil {
		return nil, err
	}
	return y.buildSelectorFn(&selNode{typ: "group", list: sel}), nil
}

// parseSelection parses until a terminator for the current context.
func parseSelection(sc *fmtScanner, insideMerge, insideChoice, insideGroup bool) ([]*selNode, string, error) {
	var selectors []*selNode
	var current *selNode
	term := ""
	for {
		typ, val, ok := sc.next()
		if !ok {
			break
		}
		if typ == "name" {
			current = &selNode{typ: "single", spec: val}
			continue
		}
		if typ == "filter" {
			if current == nil {
				current = &selNode{typ: "single", spec: "best"}
			}
			current.filters = append(current.filters, val)
			continue
		}
		// op
		switch val {
		case ")":
			if !insideGroup {
				sc.pos--
			}
			return appendSel(selectors, current), ")", nil
		case "/":
			if insideMerge || insideChoice {
				sc.pos--
				return appendSel(selectors, current), "/", nil
			}
			if current == nil {
				return nil, "", fmt.Errorf("invalid format specification: %q", sc.s)
			}
			second, _, err := parseSelection(sc, false, true, false)
			if err != nil {
				return nil, "", err
			}
			current = &selNode{typ: "pickfirst", a: current, list: second}
		case ",":
			if insideMerge || insideChoice {
				sc.pos--
				return appendSel(selectors, current), ",", nil
			}
			if current == nil {
				return nil, "", fmt.Errorf("invalid format specification: %q", sc.s)
			}
			selectors = append(selectors, current)
			current = nil
		case "+":
			if current == nil {
				return nil, "", fmt.Errorf("invalid format specification: %q", sc.s)
			}
			second, _, err := parseSelection(sc, true, false, false)
			if err != nil {
				return nil, "", err
			}
			if len(second) == 0 {
				return nil, "", fmt.Errorf("expected a selector after + in %q", sc.s)
			}
			var rhs *selNode
			if len(second) == 1 {
				rhs = second[0]
			} else {
				rhs = &selNode{typ: "group", list: second}
			}
			current = &selNode{typ: "merge", a: current, b: rhs}
		case "(":
			if current != nil {
				return nil, "", fmt.Errorf("unexpected ( in %q", sc.s)
			}
			group, _, err := parseSelection(sc, false, false, true)
			if err != nil {
				return nil, "", err
			}
			current = &selNode{typ: "group", list: group}
		default:
			return nil, "", fmt.Errorf("operator not recognized: %q", val)
		}
	}
	return appendSel(selectors, current), term, nil
}

func appendSel(list []*selNode, n *selNode) []*selNode {
	if n != nil {
		return append(list, n)
	}
	return list
}

// ---------------------------------------------------------------------------
// Evaluator
// ---------------------------------------------------------------------------

func (y *YoutubeDL) buildSelectorFn(n *selNode) selectorFn {
	switch n.typ {
	case "group":
		fs := make([]selectorFn, len(n.list))
		for i, child := range n.list {
			fs[i] = y.buildSelectorFn(child)
		}
		base := func(formats []infodict.Format) []infodict.Format {
			var out []infodict.Format
			for _, f := range fs {
				out = append(out, f(formats)...)
			}
			return out
		}
		return applyFilters(base, n.filters)
	case "pickfirst":
		first := y.buildSelectorFn(n.a)
		rest := make([]selectorFn, len(n.list))
		for i, child := range n.list {
			rest[i] = y.buildSelectorFn(child)
		}
		base := func(formats []infodict.Format) []infodict.Format {
			if got := first(formats); len(got) > 0 {
				return got
			}
			for _, f := range rest {
				if got := f(formats); len(got) > 0 {
					return got
				}
			}
			return nil
		}
		return applyFilters(base, n.filters)
	case "merge":
		s1 := y.buildSelectorFn(n.a)
		s2 := y.buildSelectorFn(n.b)
		base := func(formats []infodict.Format) []infodict.Format {
			var out []infodict.Format
			for _, f1 := range s1(formats) {
				for _, f2 := range s2(formats) {
					out = append(out, mergeFormats(f1, f2))
				}
			}
			return out
		}
		return applyFilters(base, n.filters)
	default: // single
		base := y.singleSelector(n.spec)
		return applyFilters(base, n.filters)
	}
}

func applyFilters(base selectorFn, filters []string) selectorFn {
	if len(filters) == 0 {
		return base
	}
	fns := make([]func(infodict.Format) bool, 0, len(filters))
	for _, fs := range filters {
		if fn, err := buildFormatFilter(fs); err == nil {
			fns = append(fns, fn)
		}
	}
	return func(formats []infodict.Format) []infodict.Format {
		kept := make([]infodict.Format, 0, len(formats))
		for _, f := range formats {
			ok := true
			for _, fn := range fns {
				if !fn(f) {
					ok = false
					break
				}
			}
			if ok {
				kept = append(kept, f)
			}
		}
		return base(kept)
	}
}

func (y *YoutubeDL) singleSelector(spec string) selectorFn {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "best"
	}
	lower := strings.ToLower(spec)

	if lower == "all" {
		return func(formats []infodict.Format) []infodict.Format {
			out := make([]infodict.Format, len(formats))
			for i := range formats {
				out[i] = formats[len(formats)-1-i]
			}
			return out
		}
	}

	if bw, ftype, mod, idx, ok := parseBW(lower); ok {
		formatReverse := bw == 'b'
		formatIdx := idx
		notType := ""
		if ftype == 'v' {
			notType = "a"
		} else if ftype == 'a' {
			notType = "v"
		}
		formatFallback := ftype == 0 && !mod
		filter := func(f infodict.Format) bool {
			if ftype != 0 {
				c := codecOf(f, ftype)
				if mod {
					return c != "none" && c != ""
				}
				nc := codecOf(f, rune(notType[0]))
				return nc == "none" || nc == ""
			}
			if !mod {
				return hasVideo(f) && hasAudio(f)
			}
			return true
		}
		filterAny := func(f infodict.Format) bool {
			return filter(f) && (hasVideo(f) || hasAudio(f))
		}
		return func(formats []infodict.Format) []infodict.Format {
			var matches []infodict.Format
			for _, f := range formats {
				if filterAny(f) {
					matches = append(matches, f)
				}
			}
			if len(matches) == 0 && formatFallback {
				for _, f := range formats {
					if hasVideo(f) || hasAudio(f) {
						matches = append(matches, f)
					}
				}
			}
			if formatReverse {
				reverseFormats(matches)
			}
			if formatIdx-1 < len(matches) {
				return []infodict.Format{matches[formatIdx-1]}
			}
			return nil
		}
	}

	if audioExts[lower] {
		return func(formats []infodict.Format) []infodict.Format {
			for i := len(formats) - 1; i >= 0; i-- {
				if formats[i].Ext == lower && hasAudio(formats[i]) {
					return []infodict.Format{formats[i]}
				}
			}
			return nil
		}
	}
	if videoExts[lower] {
		return func(formats []infodict.Format) []infodict.Format {
			for i := len(formats) - 1; i >= 0; i-- {
				f := formats[i]
				if f.Ext == lower && hasVideo(f) && hasAudio(f) {
					return []infodict.Format{f}
				}
			}
			for i := len(formats) - 1; i >= 0; i-- {
				f := formats[i]
				if f.Ext == lower && hasVideo(f) {
					return []infodict.Format{f}
				}
			}
			return nil
		}
	}
	if storyboardExts[lower] {
		return func(formats []infodict.Format) []infodict.Format {
			for i := len(formats) - 1; i >= 0; i-- {
				f := formats[i]
				if f.Ext == lower && !hasAudio(f) && !hasVideo(f) {
					return []infodict.Format{f}
				}
			}
			return nil
		}
	}
	// format_id
	return func(formats []infodict.Format) []infodict.Format {
		for i := len(formats) - 1; i >= 0; i-- {
			if formats[i].FormatID == spec {
				return []infodict.Format{formats[i]}
			}
		}
		return nil
	}
}

func codecOf(f infodict.Format, t rune) string {
	if t == 'v' {
		return f.VCodec
	}
	return f.ACodec
}

func reverseFormats(fs []infodict.Format) {
	for i, j := 0, len(fs)-1; i < j; i, j = i+1, j-1 {
		fs[i], fs[j] = fs[j], fs[i]
	}
}

func parseBW(spec string) (bw rune, ftype rune, mod bool, idx int, ok bool) {
	idx = 1
	if strings.HasPrefix(spec, "best") {
		bw = 'b'
		spec = spec[4:]
	} else if strings.HasPrefix(spec, "worst") {
		bw = 'w'
		spec = spec[5:]
	} else if strings.HasPrefix(spec, "b") {
		bw = 'b'
		spec = spec[1:]
	} else if strings.HasPrefix(spec, "w") {
		bw = 'w'
		spec = spec[1:]
	} else {
		return 0, 0, false, 0, false
	}
	if strings.HasPrefix(spec, "video") {
		ftype = 'v'
		spec = spec[5:]
	} else if strings.HasPrefix(spec, "audio") {
		ftype = 'a'
		spec = spec[5:]
	} else if strings.HasPrefix(spec, "v") {
		ftype = 'v'
		spec = spec[1:]
	} else if strings.HasPrefix(spec, "a") {
		ftype = 'a'
		spec = spec[1:]
	}
	if strings.HasPrefix(spec, "*") {
		mod = true
		spec = spec[1:]
	}
	if strings.HasPrefix(spec, ".") {
		n, err := strconv.Atoi(spec[1:])
		if err == nil && n >= 1 {
			idx = n
		}
		spec = ""
	}
	if spec != "" {
		return 0, 0, false, 0, false
	}
	return bw, ftype, mod, idx, true
}

// ---------------------------------------------------------------------------
// Filters
// ---------------------------------------------------------------------------

func buildFormatFilter(spec string) (func(infodict.Format) bool, error) {
	spec = strings.TrimSpace(spec)
	ops := []string{"<=", ">=", "!=", "=", "<", ">"}
	for _, op := range ops {
		if i := strings.Index(spec, op); i > 0 {
			key := strings.TrimSpace(spec[:i])
			val := strings.TrimSpace(spec[i+len(op):])
			noneInclusive := strings.HasPrefix(val, "?")
			val = strings.TrimPrefix(val, "?")
			cmp, err := parseFilterValue(val)
			if err != nil {
				return nil, err
			}
			return func(f infodict.Format) bool {
				v, present := filterFieldValue(f, key)
				if !present {
					return noneInclusive
				}
				return applyOp(op, v, cmp)
			}, nil
		}
	}
	return nil, fmt.Errorf("invalid filter specification %q", spec)
}

func parseFilterValue(v string) (float64, error) {
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f, nil
	}
	return 0, fmt.Errorf("invalid filter value %q", v)
}

func filterFieldValue(f infodict.Format, key string) (float64, bool) {
	switch key {
	case "height":
		return float64(f.Height), f.Height != 0
	case "width":
		return float64(f.Width), f.Width != 0
	case "fps":
		return f.FPS, f.FPS != 0
	case "tbr", "vbr", "abr":
		if key == "tbr" {
			return f.TBR, f.TBR != 0
		}
		if key == "vbr" {
			return f.VBR, f.VBR != 0
		}
		return f.ABR, f.ABR != 0
	case "filesize":
		return float64(f.Filesize), f.Filesize != 0
	case "filesize_approx":
		return float64(f.FilesizeApprox), f.FilesizeApprox != 0
	case "asr":
		return float64(f.ASR), f.ASR != 0
	case "audio_channels":
		return float64(f.AudioChannels), f.AudioChannels != 0
	}
	return 0, false
}

func applyOp(op string, a, b float64) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "=":
		return a == b
	case "!=":
		return a != b
	}
	return false
}

// ---------------------------------------------------------------------------
// Merge
// ---------------------------------------------------------------------------

func mergeFormats(f1, f2 infodict.Format) infodict.Format {
	formats := []infodict.Format{}
	if len(f1.RequestedFormats) > 0 {
		formats = append(formats, f1.RequestedFormats...)
	} else {
		formats = append(formats, f1)
	}
	if len(f2.RequestedFormats) > 0 {
		formats = append(formats, f2.RequestedFormats...)
	} else {
		formats = append(formats, f2)
	}

	if len(formats) == 1 {
		return formats[0]
	}

	var videoFmts, audioFmts []infodict.Format
	ids := make([]string, 0, len(formats))
	notes := []string{}
	var tbr float64
	var sizeApprox int64
	for _, f := range formats {
		if hasVideo(f) {
			videoFmts = append(videoFmts, f)
		}
		if hasAudio(f) {
			audioFmts = append(audioFmts, f)
		}
		if f.FormatID != "" {
			ids = append(ids, f.FormatID)
		}
		if f.FormatNote != "" {
			notes = append(notes, f.FormatNote)
		}
		tbr += f.TBR
		sizeApprox += f.FilesizeApprox
	}

	out := infodict.Format{
		RequestedFormats: formats,
		FormatID:         strings.Join(ids, "+"),
		Format:           strings.Join(ids, "+"),
		Ext:              getCompatibleExt(formats),
		TBR:              tbr,
		FilesizeApprox:   sizeApprox,
	}
	if len(notes) > 0 {
		out.FormatNote = strings.Join(dedupe(notes), "+")
	}
	if len(videoFmts) == 1 {
		v := videoFmts[0]
		out.Width = v.Width
		out.Height = v.Height
		out.Resolution = formatResolution(v)
		out.FPS = v.FPS
		out.VCodec = v.VCodec
		out.VBR = v.VBR
		out.DynamicRange = v.DynamicRange
	}
	if len(audioFmts) == 1 {
		a := audioFmts[0]
		out.ACodec = a.ACodec
		out.ABR = a.ABR
		out.ASR = a.ASR
		out.AudioChannels = a.AudioChannels
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func getCompatibleExt(formats []infodict.Format) string {
	var vexts, aexts []string
	var vcodecs, acodecs []string
	for _, f := range formats {
		if hasVideo(f) {
			vexts = append(vexts, f.Ext)
			vcodecs = append(vcodecs, f.VCodec)
		}
		if hasAudio(f) {
			aexts = append(aexts, f.Ext)
			acodecs = append(acodecs, f.ACodec)
		}
	}
	allWebm := len(vexts) > 0
	for _, e := range vexts {
		if e != "webm" {
			allWebm = false
		}
	}
	for _, e := range aexts {
		if e != "webm" && e != "weba" && e != "opus" && e != "ogg" {
			allWebm = false
		}
	}
	if allWebm {
		return "webm"
	}
	_ = vcodecs
	_ = acodecs
	return "mp4"
}
