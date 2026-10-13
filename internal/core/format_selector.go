package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
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

// selectContext mirrors yt-dlp's selector context (see YoutubeDL._select_formats).
type selectContext struct {
	formats           []infodict.Format
	hasMergedFormat   bool
	incompleteFormats bool
}

// selectorFn picks formats from a selection context.
type selectorFn func(ctx *selectContext) []infodict.Format

func hasVideo(f infodict.Format) bool { return f.VCodec != "" && f.VCodec != "none" }
func hasAudio(f infodict.Format) bool { return f.ACodec != "" && f.ACodec != "none" }

// selectFormats applies a format specification to the given formats.
func (y *YoutubeDL) selectFormats(formats []infodict.Format, spec string) ([]infodict.Format, error) {
	fn, err := y.buildFormatSelector(spec)
	if err != nil {
		return nil, err
	}
	allNoVideo, allNoAudio := true, true
	for _, f := range formats {
		if hasVideo(f) {
			allNoVideo = false
		}
		if hasAudio(f) {
			allNoAudio = false
		}
	}
	ctx := &selectContext{
		formats:           formats,
		hasMergedFormat:   hasVideoAndAudioAny(formats),
		incompleteFormats: allNoVideo || allNoAudio,
	}
	return fn(ctx), nil
}

func hasVideoAndAudioAny(formats []infodict.Format) bool {
	for _, f := range formats {
		if hasVideo(f) && hasAudio(f) {
			return true
		}
	}
	return false
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
		var quote byte
		for sc.pos < len(sc.s) {
			ch := sc.s[sc.pos]
			if quote != 0 {
				if ch == '\\' && sc.pos+1 < len(sc.s) {
					sc.pos += 2
					continue
				}
				if ch == quote {
					quote = 0
				}
			} else if ch == '"' || ch == '\'' {
				quote = ch
			} else if ch == ']' {
				break
			}
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
	return y.buildSelectorFn(&selNode{typ: "group", list: sel})
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
			current.filters = append(current.filters, splitFormatFilters(val)...)
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

func (y *YoutubeDL) buildSelectorFn(n *selNode) (selectorFn, error) {
	switch n.typ {
	case "group":
		fs := make([]selectorFn, len(n.list))
		for i, child := range n.list {
			f, err := y.buildSelectorFn(child)
			if err != nil {
				return nil, err
			}
			fs[i] = f
		}
		base := func(ctx *selectContext) []infodict.Format {
			var out []infodict.Format
			for _, f := range fs {
				out = append(out, f(ctx)...)
			}
			return out
		}
		return applyFilters(base, n.filters)
	case "pickfirst":
		first, err := y.buildSelectorFn(n.a)
		if err != nil {
			return nil, err
		}
		rest := make([]selectorFn, len(n.list))
		for i, child := range n.list {
			f, err := y.buildSelectorFn(child)
			if err != nil {
				return nil, err
			}
			rest[i] = f
		}
		base := func(ctx *selectContext) []infodict.Format {
			if got := first(ctx); len(got) > 0 {
				return got
			}
			for _, f := range rest {
				if got := f(ctx); len(got) > 0 {
					return got
				}
			}
			return nil
		}
		return applyFilters(base, n.filters)
	case "merge":
		s1, err := y.buildSelectorFn(n.a)
		if err != nil {
			return nil, err
		}
		s2, err := y.buildSelectorFn(n.b)
		if err != nil {
			return nil, err
		}
		base := func(ctx *selectContext) []infodict.Format {
			prefs := y.mergePreferences()
			var out []infodict.Format
			for _, f1 := range s1(ctx) {
				for _, f2 := range s2(ctx) {
					out = append(out, mergeFormats(f1, f2, prefs))
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

func applyFilters(base selectorFn, filters []string) (selectorFn, error) {
	if len(filters) == 0 {
		return base, nil
	}
	fns := make([]func(infodict.Format) bool, 0, len(filters))
	for _, fs := range filters {
		fn, err := buildFormatFilter(fs)
		if err != nil {
			return nil, err
		}
		fns = append(fns, fn)
	}
	return func(ctx *selectContext) []infodict.Format {
		kept := make([]infodict.Format, 0, len(ctx.formats))
		for _, f := range ctx.formats {
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
		ctx.formats = kept
		return base(ctx)
	}, nil
}

func (y *YoutubeDL) singleSelector(spec string) selectorFn {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "best"
	}
	lower := strings.ToLower(spec)

	if lower == "all" {
		return func(ctx *selectContext) []infodict.Format {
			out := make([]infodict.Format, len(ctx.formats))
			for i := range ctx.formats {
				out[i] = ctx.formats[len(ctx.formats)-1-i]
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
		return func(ctx *selectContext) []infodict.Format {
			var matches []infodict.Format
			for _, f := range ctx.formats {
				if filterAny(f) {
					matches = append(matches, f)
				}
			}
			if len(matches) == 0 && formatFallback && ctx.incompleteFormats {
				for _, f := range ctx.formats {
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
		return func(ctx *selectContext) []infodict.Format {
			for i := len(ctx.formats) - 1; i >= 0; i-- {
				if ctx.formats[i].Ext == lower && hasAudio(ctx.formats[i]) {
					return []infodict.Format{ctx.formats[i]}
				}
			}
			return nil
		}
	}
	if videoExts[lower] {
		return func(ctx *selectContext) []infodict.Format {
			for i := len(ctx.formats) - 1; i >= 0; i-- {
				f := ctx.formats[i]
				if f.Ext == lower && hasVideo(f) && hasAudio(f) {
					return []infodict.Format{f}
				}
			}
			if !ctx.hasMergedFormat {
				for i := len(ctx.formats) - 1; i >= 0; i-- {
					f := ctx.formats[i]
					if f.Ext == lower && hasVideo(f) {
						return []infodict.Format{f}
					}
				}
			}
			return nil
		}
	}
	if storyboardExts[lower] {
		return func(ctx *selectContext) []infodict.Format {
			for i := len(ctx.formats) - 1; i >= 0; i-- {
				f := ctx.formats[i]
				if f.Ext == lower && !hasAudio(f) && !hasVideo(f) {
					return []infodict.Format{f}
				}
			}
			return nil
		}
	}
	// format_id
	return func(ctx *selectContext) []infodict.Format {
		for i := len(ctx.formats) - 1; i >= 0; i-- {
			if ctx.formats[i].FormatID == spec {
				return []infodict.Format{ctx.formats[i]}
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

// numFilterRe mirrors yt-dlp's numeric filter regex.
var numFilterRe = regexp.MustCompile(`^\s*([\w.-]+)\s*(<=|>=|!=|=|<|>)\s*(\?)?\s*([0-9.]+(?:[kKmMgGtTpPeEzZyY]i?[Bb]?)?)\s*$`)

func buildFormatFilter(spec string) (func(infodict.Format) bool, error) {
	if fn, ok := numericFilter(spec); ok {
		return fn, nil
	}
	if fn, ok := stringFilter(spec); ok {
		return fn, nil
	}
	return nil, fmt.Errorf("invalid filter specification %q", strings.TrimSpace(spec))
}

// splitFormatFilters splits a filter bracket's content on top-level commas,
// honouring quoted sections.
func splitFormatFilters(s string) []string {
	var out []string
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case ',':
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

// numericFilter reproduces yt-dlp's numeric comparison filters. It returns
// ok=false when the spec does not match the numeric grammar (so a string
// operator can be attempted instead).
func numericFilter(spec string) (func(infodict.Format) bool, bool) {
	m := numFilterRe.FindStringSubmatch(spec)
	if m == nil {
		return nil, false
	}
	key, op := m[1], m[2]
	noneInclusive := m[3] == "?"
	num, ok := parseFilterNumber(m[4])
	if !ok {
		return nil, false
	}
	return func(f infodict.Format) bool {
		v, present := filterAttr(f, key)
		if !present {
			return noneInclusive
		}
		n, isNum := toFloat(v)
		switch op {
		case "=":
			return isNum && n == num
		case "!=":
			return !isNum || n != num
		case "<":
			return isNum && n < num
		case "<=":
			return isNum && n <= num
		case ">":
			return isNum && n > num
		case ">=":
			return isNum && n >= num
		}
		return false
	}, true
}

// stringFilter reproduces yt-dlp's string comparison filters, including the
// negation prefix (!) and the none-inclusive marker (?).
func stringFilter(spec string) (func(infodict.Format) bool, bool) {
	i := skipSpaces(spec, 0)
	start := i
	for i < len(spec) && isFilterKeyChar(spec[i]) {
		i++
	}
	if i == start {
		return nil, false
	}
	key := spec[start:i]
	i = skipSpaces(spec, i)

	neg := false
	if i < len(spec) && spec[i] == '!' {
		neg = true
		i = skipSpaces(spec, i+1)
	}

	var op string
	switch {
	case strings.HasPrefix(spec[i:], "^="), strings.HasPrefix(spec[i:], "$="),
		strings.HasPrefix(spec[i:], "*="), strings.HasPrefix(spec[i:], "~="):
		op = spec[i : i+2]
		i += 2
	case i < len(spec) && spec[i] == '=':
		op = "="
		i++
	default:
		return nil, false
	}
	i = skipSpaces(spec, i)

	noneInclusive := false
	if i < len(spec) && spec[i] == '?' {
		noneInclusive = true
		i = skipSpaces(spec, i+1)
	}

	var val string
	if i < len(spec) && (spec[i] == '"' || spec[i] == '\'') {
		q := spec[i]
		i++
		var b strings.Builder
		closed := false
		for i < len(spec) {
			c := spec[i]
			if c == '\\' && i+1 < len(spec) {
				nxt := spec[i+1]
				if nxt == '\\' || nxt == '"' || nxt == '\'' {
					b.WriteByte(nxt)
					i += 2
					continue
				}
			}
			if c == q {
				i++
				closed = true
				break
			}
			b.WriteByte(c)
			i++
		}
		if !closed {
			return nil, false
		}
		val = b.String()
	} else {
		vstart := i
		for i < len(spec) && isFilterValueChar(spec[i]) {
			i++
		}
		if i == vstart {
			return nil, false
		}
		val = spec[vstart:i]
	}
	if skipSpaces(spec, i) != len(spec) {
		return nil, false
	}

	var re *regexp.Regexp
	if op == "~=" {
		compiled, err := regexp.Compile(val)
		if err != nil {
			return nil, false
		}
		re = compiled
	}

	return func(f infodict.Format) bool {
		v, present := filterAttr(f, key)
		if !present {
			return noneInclusive
		}
		sv := asFilterString(v)
		var res bool
		switch op {
		case "=":
			res = sv == val
		case "^=":
			res = strings.HasPrefix(sv, val)
		case "$=":
			res = strings.HasSuffix(sv, val)
		case "*=":
			res = strings.Contains(sv, val)
		case "~=":
			res = re.MatchString(sv)
		}
		if neg {
			res = !res
		}
		return res
	}, true
}

func parseFilterNumber(v string) (float64, bool) {
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		return n, true
	}
	if n, ok := utils.ParseBytes(v); ok {
		return float64(n), true
	}
	return 0, false
}

func skipSpaces(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func isFilterKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
}

func isFilterValueChar(c byte) bool {
	return isFilterKeyChar(c)
}

func asFilterString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// filterAttr returns the value of a format field by its yt-dlp key together
// with whether the field is present (non-zero / non-empty), matching the
// omitempty behaviour of the serialized info dict.
func filterAttr(f infodict.Format, key string) (any, bool) {
	switch key {
	case "format_id":
		return strAttr(f.FormatID)
	case "format":
		return strAttr(f.Format)
	case "format_note":
		return strAttr(f.FormatNote)
	case "ext":
		return strAttr(f.Ext)
	case "width":
		return intAttr(int64(f.Width))
	case "height":
		return intAttr(int64(f.Height))
	case "resolution":
		return strAttr(f.Resolution)
	case "aspect_ratio":
		return floatPtrAttr(f.AspectRatio)
	case "fps":
		return floatAttr(f.FPS)
	case "tbr":
		return floatAttr(f.TBR)
	case "vbr":
		return floatPtrAttr(f.VBR)
	case "abr":
		return floatPtrAttr(f.ABR)
	case "asr":
		return intAttr(int64(f.ASR))
	case "audio_channels":
		return intAttr(int64(f.AudioChannels))
	case "filesize":
		return intAttr(f.Filesize)
	case "filesize_approx":
		return intAttr(f.FilesizeApprox)
	case "acodec":
		return strAttr(f.ACodec)
	case "vcodec":
		return strAttr(f.VCodec)
	case "dynamic_range":
		return strAttr(f.DynamicRange)
	case "language":
		return strAttr(f.Language)
	case "container":
		return strAttr(f.Container)
	case "protocol":
		return strAttr(f.Protocol)
	case "url":
		return strAttr(f.URL)
	case "manifest_url":
		return strAttr(f.ManifestURL)
	case "preference":
		return intPtrAttr(f.Preference)
	case "quality":
		return floatPtrAttr(f.Quality)
	case "source_preference":
		return intPtrAttr(f.SourcePreference)
	case "language_preference":
		return intPtrAttr(f.LanguagePreference)
	case "has_drm":
		if f.HasDRM == nil {
			return nil, false
		}
		return f.HasDRM, true
	case "is_live":
		if f.IsLive == nil {
			return nil, false
		}
		return *f.IsLive, true
	}
	return nil, false
}

func strAttr(s string) (any, bool) {
	if s == "" {
		return nil, false
	}
	return s, true
}

func intAttr(n int64) (any, bool) {
	if n == 0 {
		return nil, false
	}
	return n, true
}

func floatAttr(x float64) (any, bool) {
	if x == 0 {
		return nil, false
	}
	return x, true
}

func intPtrAttr(p *int) (any, bool) {
	if p == nil {
		return nil, false
	}
	return int64(*p), true
}

func floatPtrAttr(p *float64) (any, bool) {
	if p == nil {
		return nil, false
	}
	return *p, true
}

// ---------------------------------------------------------------------------
// Merge
// ---------------------------------------------------------------------------

func mergeFormats(f1, f2 infodict.Format, preferences []string) infodict.Format {
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
		Format:           joinFormatStrings(formats, ids),
		Ext:              getCompatibleExt(formats, preferences),
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
		out.AspectRatio = v.AspectRatio
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

// joinFormatStrings builds the "format" string of a merged format as yt-dlp
// does: the derived format strings of each component joined with '+'.
func joinFormatStrings(formats []infodict.Format, ids []string) string {
	parts := make([]string, 0, len(formats))
	for _, f := range formats {
		s := f.Format
		if s == "" {
			s = f.FormatID
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	if s := strings.Join(parts, "+"); s != "" {
		return s
	}
	return strings.Join(ids, "+")
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

// mergePreferences returns the --merge-output-format containers as a list, or
// nil when unset (which preserves the project's default mkv behaviour).
func (y *YoutubeDL) mergePreferences() []string {
	s := strings.TrimSpace(y.opts.MergeOutputFormat)
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

// getCompatibleExt picks the container extension for a merged format,
// mirroring yt-dlp's utils.get_compatible_ext. When preferences is empty the
// project default applies and the 'webm' default is replaced by 'mkv'; when
// preferences is set the containers are returned verbatim.
func getCompatibleExt(formats []infodict.Format, preferences []string) string {
	var vexts, aexts, vcodecs, acodecs []string
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
	if len(preferences) > 0 {
		return preferredCompatibleExt(preferences, vexts, aexts, vcodecs, acodecs)
	}

	if maxInt(len(acodecs), len(vcodecs)) > 1 {
		return "mkv"
	}

	vcodec := sanitizeCodec(vcodecs)
	acodec := sanitizeCodec(acodecs)
	for _, c := range compatibleCodecOrder {
		if containsStr(compatibleCodecs[c], vcodec) && containsStr(compatibleCodecs[c], acodec) {
			return mergedContainer(c)
		}
	}

	compatibleExts := [][]string{
		{"mp3", "mp4", "m4a", "m4p", "m4b", "m4r", "m4v", "ismv", "isma", "mov"},
		{"webm", "weba"},
	}
	for _, ext := range vexts {
		current := map[string]bool{ext: true}
		for _, e := range vexts {
			current[e] = true
		}
		for _, e := range aexts {
			current[e] = true
		}
		if len(current) == 1 {
			return mergedContainer(ext)
		}
		for _, set := range compatibleExts {
			if isSuperset(set, current) {
				return mergedContainer(ext)
			}
		}
	}
	return "mkv"
}

// compatibleCodecs reproduces yt-dlp's COMPATIBLE_CODECS table.
var compatibleCodecs = map[string][]string{
	"mp4":  {"av1", "hevc", "avc1", "mp4a", "ac-4", "h264", "aacl", "ec-3"},
	"webm": {"av1", "vp9", "vp8", "opus", "vrbs", "vp9x", "vp8x"},
}

// compatibleCodecOrder is the iteration order of COMPATIBLE_CODECS (insertion
// order in yt-dlp: mp4 then webm).
var compatibleCodecOrder = []string{"mp4", "webm"}

// preferredCompatibleExt ports yt-dlp's get_compatible_ext when an explicit
// --merge-output-format preference list is given. Unlike the default path it
// returns the containers verbatim (no mkv remapping).
func preferredCompatibleExt(preferences, vexts, aexts, vcodecs, acodecs []string) string {
	allowMkv := false
	for _, p := range preferences {
		if p == "mkv" {
			allowMkv = true
			break
		}
	}
	if allowMkv && maxInt(len(acodecs), len(vcodecs)) > 1 {
		return "mkv"
	}

	vcodec := sanitizeCodec(vcodecs)
	acodec := sanitizeCodec(acodecs)
	for _, ext := range preferences {
		if ext == "mkv" {
			return ext
		}
		if codecs, ok := compatibleCodecs[ext]; ok && containsStr(codecs, vcodec) && containsStr(codecs, acodec) {
			return ext
		}
	}

	compatibleExtSets := [][]string{
		{"mp3", "mp4", "m4a", "m4p", "m4b", "m4r", "m4v", "ismv", "isma", "mov"},
		{"webm", "weba"},
	}
	for _, ext := range preferences {
		current := map[string]bool{ext: true}
		for _, e := range vexts {
			current[e] = true
		}
		for _, e := range aexts {
			current[e] = true
		}
		if ext == "mkv" || len(current) == 1 || anySuperset(compatibleExtSets, current) {
			return ext
		}
	}

	if allowMkv {
		return "mkv"
	}
	return preferences[len(preferences)-1]
}

func anySuperset(sets [][]string, subset map[string]bool) bool {
	for _, set := range sets {
		if isSuperset(set, subset) {
			return true
		}
	}
	return false
}

// mergedContainer maps the yt-dlp-compatible container to this project's
// default merged output: webm/weba become mkv.
func mergedContainer(ext string) string {
	if ext == "webm" || ext == "weba" {
		return "mkv"
	}
	return ext
}

// sanitizeCodec reproduces yt-dlp's codec normalization: take the first codec,
// strip everything after the first dot, drop '0' runes and lowercase.
func sanitizeCodec(codecs []string) string {
	if len(codecs) == 0 {
		return ""
	}
	first := codecs[0]
	if i := strings.IndexByte(first, '.'); i >= 0 {
		first = first[:i]
	}
	return strings.ToLower(strings.ReplaceAll(first, "0", ""))
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func isSuperset(set []string, subset map[string]bool) bool {
	for k := range subset {
		if !containsStr(set, k) {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
