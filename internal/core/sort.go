package core

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// formatSorter is the Go port of yt-dlp's utils.FormatSorter.
type formatSorter struct {
	order     []string
	settings  map[string]*sortSetting
	useFree   bool
	sortUser  []string
	sortExtra []string
}

type sortSetting struct {
	typ       string
	field     string
	fields    []string
	convert   string
	ordered   bool
	reverse   bool
	closest   bool
	limitText string
	hasLimit  bool
	limit     float64
	order     []string
	orderFree []string
	regex     bool
	inList    []string
	notInList []string
	max       float64
	hasMax    bool
	def       float64
	hasDef    bool
	fn        func([]any) any
}

var sortRegex = regexp.MustCompile(` *((?P<reverse>\+)?(?P<field>[a-zA-Z0-9_]+)((?P<separator>[~:])(?P<limit>.*?))?)? *$`)

var defaultSortFields = []string{
	"hidden", "aud_or_vid", "hasvid", "ie_pref", "lang", "quality",
	"res", "fps", "hdr:12", "vcodec", "channels", "acodec",
	"size", "br", "asr", "proto", "ext", "hasaud", "source", "id",
}

func newFormatSorter(opts *optionsSortParams) *formatSorter {
	s := &formatSorter{settings: map[string]*sortSetting{}}
	s.useFree = opts.preferFree
	s.sortUser = opts.sortUser
	s.sortExtra = opts.sortExtra

	// forced fields first, then priority fields, then user/extractor/default
	var list []string
	for _, f := range defaultSortFields {
		if isForcedField(f) {
			list = append(list, f)
		}
	}
	if !opts.forceUser {
		for _, f := range defaultSortFields {
			if isPriorityField(f) {
				list = append(list, f)
			}
		}
	}
	list = append(list, s.sortUser...)
	list = append(list, s.sortExtra...)
	list = append(list, defaultSortFields...)

	for _, item := range list {
		m := sortRegex.FindStringSubmatch(item)
		if m == nil {
			continue
		}
		field := m[sortRegex.SubexpIndex("field")]
		if field == "" {
			continue
		}
		field = strings.ToLower(field)
		if s.getType(field) == "alias" {
			field = s.getSetting(field, "field").(string)
		}
		if s.contains(field) {
			continue
		}
		s.order = append(s.order, field)
		set := s.settings[field]
		if set == nil {
			set = &sortSetting{}
			s.settings[field] = set
		}
		set.reverse = m[sortRegex.SubexpIndex("reverse")] != ""
		set.closest = m[sortRegex.SubexpIndex("separator")] == "~"
		lt := m[sortRegex.SubexpIndex("limit")]
		if lt != "" {
			set.limitText = lt
			set.hasLimit = true
			set.limit = s.resolveFieldValue(field, ptrString(lt), false)
		}
	}
	return s
}

type optionsSortParams struct {
	preferFree bool
	sortUser   []string
	sortExtra  []string
	forceUser  bool
}

func (s *formatSorter) contains(f string) bool {
	for _, x := range s.order {
		if x == f {
			return true
		}
	}
	return false
}

func isForcedField(field string) bool {
	return field == "hidden" || field == "aud_or_vid"
}

func isPriorityField(field string) bool {
	return field == "ie_pref" || field == "hasvid"
}

func (s *formatSorter) getType(field string) string {
	switch field {
	case "hidden":
		return "extractor"
	case "aud_or_vid":
		return "multiple"
	case "ie_pref":
		return "extractor"
	case "hasvid", "hasaud":
		return "boolean"
	case "codec":
		return "combined"
	case "br", "size", "res":
		return "multiple"
	case "ext":
		return "combined"
	case "vext", "aext", "vcodec", "acodec", "hdr", "proto":
		return "ordered"
	}
	switch field {
	case "format_id", "preference", "language_preference", "source_preference",
		"protocol", "filesize_approx", "audio_channels":
		return "alias"
	}
	return "field"
}

func (s *formatSorter) getSetting(field, key string) any {
	switch key {
	case "forced":
		return field == "hidden" || field == "aud_or_vid"
	case "priority":
		return field == "ie_pref" || field == "hasvid"
	}
	switch field {
	case "hidden":
		if key == "max" {
			return float64(-1000)
		}
		if key == "typ" {
			return "extractor"
		}
	case "aud_or_vid":
		if key == "typ" {
			return "multiple"
		}
		if key == "fields" {
			return []string{"vcodec", "acodec"}
		}
	case "ie_pref":
		if key == "field" {
			return "preference"
		}
	case "hasvid":
		if key == "field" {
			return "vcodec"
		}
	case "hasaud":
		if key == "field" {
			return "acodec"
		}
	case "lang":
		if key == "field" {
			return "language_preference"
		}
		if key == "default" {
			return float64(-1)
		}
	case "source":
		if key == "field" {
			return "source_preference"
		}
		if key == "default" {
			return float64(-1)
		}
	case "channels":
		if key == "field" {
			return "audio_channels"
		}
	case "id":
		if key == "field" {
			return "format_id"
		}
	case "alias":
		// handled by getType
	}
	// alias resolution
	switch field {
	case "format_id":
		if key == "field" {
			return "id"
		}
	case "preference":
		if key == "field" {
			return "ie_pref"
		}
	case "language_preference":
		if key == "field" {
			return "lang"
		}
	case "source_preference":
		if key == "field" {
			return "source"
		}
	case "protocol":
		if key == "field" {
			return "proto"
		}
	case "filesize_approx":
		if key == "field" {
			return "fs_approx"
		}
	case "audio_channels":
		if key == "field" {
			return "channels"
		}
	}
	if key == "manual" {
		return true
	}
	return nil
}

func (s *formatSorter) resolveFieldValue(field string, v *string, convertNone bool) float64 {
	typ := s.getType(field)
	convert := "float_string"
	if typ == "ordered" {
		convert = "order"
	}
	if typ == "extractor" || typ == "boolean" || typ == "multiple" {
		convert = "float"
	}
	switch convert {
	case "order":
		orderList := s.orderList(field)
		length := len(orderList)
		emptyPos := length + 1
		for i, o := range orderList {
			if o == "" {
				emptyPos = i
			}
		}
		if v != nil && s.isRegex(field) {
			for i, rx := range orderList {
				if rx == "" {
					continue
				}
				if ok, _ := regexp.MatchString(rx, *v); ok {
					return float64(length - i)
				}
			}
			return float64(length - emptyPos)
		}
		if v != nil {
			for i, o := range orderList {
				if o == *v {
					return float64(length - i)
				}
			}
		}
		return float64(length - emptyPos)
	default:
		if v == nil {
			return 0
		}
		if f, err := strconv.ParseFloat(*v, 64); err == nil {
			return f
		}
		return 0
	}
}

func (s *formatSorter) orderList(field string) []string {
	f := field
	if f == "hdr" {
		f = "hdr"
	}
	lists := map[string][]string{
		"vcodec": {"av0?1", `vp0?9\.0?2`, "vp0?9", "[hx]265|he?vc?", "[hx]264|avc", "vp0?8", "mp4v|h263", "theora", "", "none"},
		"acodec": {"[af]lac", "wav|aiff", "opus", "vorbis|ogg", "aac", "mp?4a?", "mp3", "ac-?4", "e-?a?c-?3", "ac-?3", "dts", "", "none"},
		"hdr":    {"dv", "(hdr)?12", `(hdr)?10\+`, "(hdr)?10", "hlg", "", "sdr", "none"},
		"proto":  {"(ht|f)tps", "(ht|f)tp$", "m3u8.*", ".*dash", "websocket_frag", "rtmpe?", "", "ws|websocket", "f4"},
		"vext":   {"mp4", "mov", "webm", "flv", "", "none"},
		"aext":   {"m4a", "aac", "mp3", "ogg", "opus", "web[am]", "", "none"},
	}
	return lists[f]
}

func (s *formatSorter) isRegex(field string) bool {
	switch field {
	case "vcodec", "acodec", "hdr", "proto", "aext":
		return true
	}
	return false
}

// getFieldValue returns the raw value (as a comparable) for a field on a format.
func formatFieldValue(f *infodict.Format, field string) any {
	switch field {
	case "hidden":
		return nil
	case "aud_or_vid":
		return hasCodec(f.VCodec) || hasCodec(f.ACodec)
	case "ie_pref":
		return intPtr(f.Preference)
	case "hasvid":
		return hasCodec(f.VCodec)
	case "hasaud":
		return hasCodec(f.ACodec)
	case "lang":
		return intPtr(f.LanguagePreference)
	case "quality":
		return floatPtr(f.Quality)
	case "res":
		return minPositive(f.Height, f.Width)
	case "fps":
		return f.FPS
	case "hdr":
		return f.DynamicRange
	case "vcodec":
		return f.VCodec
	case "acodec":
		return f.ACodec
	case "channels":
		return f.AudioChannels
	case "size":
		return firstNonZero(f.Filesize, f.FilesizeApprox)
	case "br":
		return f.TBR
	case "asr":
		return f.ASR
	case "proto":
		return f.Protocol
	case "ext":
		return f.Ext
	case "source":
		return intPtr(f.SourcePreference)
	case "id":
		return f.FormatID
	}
	return nil
}

func minPositive(a, b int) float64 {
	switch {
	case a > 0 && b > 0:
		if a < b {
			return float64(a)
		}
		return float64(b)
	case a > 0:
		return float64(a)
	case b > 0:
		return float64(b)
	}
	return 0
}

func firstNonZero(a, b int64) float64 {
	if a != 0 {
		return float64(a)
	}
	return float64(b)
}

func intPtr(p *int) float64 {
	if p == nil {
		return -1
	}
	return float64(*p)
}

func floatPtr(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func hasCodec(c string) bool { return c != "" && c != "none" }

// calculatePreference returns the ordering key for a format.
func (s *formatSorter) calculatePreference(f *infodict.Format) []float64 {
	fillSortingFields(f)
	key := make([]float64, 0, len(s.order)*3)
	for _, field := range s.order {
		key = append(key, s.fieldPreference(field, f)...)
	}
	return key
}

func (s *formatSorter) fieldPreference(field string, f *infodict.Format) []float64 {
	typ := s.getType(field)
	set := s.settings[field]
	var value any
	if typ == "multiple" {
		if field == "aud_or_vid" {
			value = formatFieldValue(f, "aud_or_vid")
		} else if field == "res" {
			value = minPositive(f.Height, f.Width)
		} else if field == "br" {
			value = f.TBR
		} else if field == "size" {
			value = firstNonZero(f.Filesize, f.FilesizeApprox)
		}
	} else if typ == "combined" {
		if field == "codec" {
			value = f.VCodec
		} else if field == "ext" {
			value = f.Ext
		}
	} else {
		value = formatFieldValue(f, field)
	}
	return s.prefFromValue(field, typ, value, set)
}

func (s *formatSorter) prefFromValue(field, typ string, value any, set *sortSetting) []float64 {
	reverse, closest := false, false
	limit := 0.0
	hasLimit := false
	if set != nil {
		reverse, closest = set.reverse, set.closest
		limit, hasLimit = set.limit, set.hasLimit
	}

	if typ == "extractor" {
		v := -1.0
		if n, ok := toFloat(value); ok {
			max := -1000.0
			if field == "hidden" {
				max = -1000
			}
			if n >= max {
				v = -1
			} else {
				v = n
			}
		}
		return pad(v)
	}
	if typ == "boolean" || field == "aud_or_vid" {
		b := false
		if bv, ok := value.(bool); ok {
			b = bv
		}
		v := 0.0
		if !b {
			v = -1
		}
		return pad(v)
	}
	if typ == "ordered" {
		var str *string
		if value != nil {
			sv := strings.ToLower(strOf(value))
			str = &sv
		}
		v := s.resolveFieldValue(field, str, true)
		return pad(v)
	}

	// numeric/string field
	if value == nil {
		return []float64{-10, 0, 0}
	}
	if n, ok := toFloat(value); ok {
		switch {
		case closest && hasLimit:
			diff := n - limit
			v2 := limit - n
			if reverse {
				v2 = diff
			}
			return []float64{0, -absVal(diff), v2}
		case !reverse && (!hasLimit || n <= limit):
			return []float64{0, n, 0}
		case !hasLimit || (reverse && n == limit) || n > limit:
			return []float64{0, -n, 0}
		default:
			return []float64{-1, n, 0}
		}
	}
	// non numeric strings sort higher
	return []float64{1, 0, 0}
}

func pad(v float64) []float64 { return []float64{v, 0, 0} }

func strOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	}
	return ""
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func absVal(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func ptrString(s string) *string { return &s }

// fillSortingFields computes derived fields used by the sorter.
func fillSortingFields(f *infodict.Format) {
	if f.Protocol == "" {
		f.Protocol = determineProtocol(f)
	}
	if f.Ext == "" && f.URL != "" {
		f.Ext = extFromURL(f.URL)
	}
	if f.VCodec == "none" {
		if f.ACodec != "none" {
			f.AudioExt = f.Ext
		} else {
			f.AudioExt = "none"
		}
		f.VideoExt = "none"
	} else {
		f.VideoExt = f.Ext
		f.AudioExt = "none"
	}
	if f.VCodec == "none" {
		f.VBR = 0
	}
	if f.ACodec == "none" {
		f.ABR = 0
	}
	if f.VBR == 0 && f.VCodec != "none" {
		if f.TBR != 0 {
			f.VBR = f.TBR - f.ABR
		}
	}
	if f.ABR == 0 && f.ACodec != "none" {
		if f.TBR != 0 {
			f.ABR = f.TBR - f.VBR
		}
	}
	if f.TBR == 0 {
		f.TBR = f.VBR + f.ABR
	}
}

func absInt(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// determineProtocol guesses the download protocol of a format.
func determineProtocol(f *infodict.Format) string {
	if len(f.Fragments) > 0 {
		if f.Ext == "m4s" || f.Container == "mp4_dash" {
			return "http_dash_segments"
		}
		return "http_dash_segments"
	}
	if i := strings.Index(f.URL, "://"); i > 0 {
		return f.URL[:i]
	}
	return "http"
}

// extFromURL returns the file extension from a URL path.
func extFromURL(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	if i := strings.IndexByte(u, '#'); i >= 0 {
		u = u[:i]
	}
	base := u
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		base = u[i+1:]
	}
	if i := strings.LastIndexByte(base, '.'); i >= 0 && i+1 < len(base) {
		return strings.ToLower(base[i+1:])
	}
	return "mp4"
}
