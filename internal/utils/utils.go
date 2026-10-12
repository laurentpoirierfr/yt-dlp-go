// Package utils ports the general-purpose helpers of yt_dlp/utils.
package utils

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	reIntOnly   = regexp.MustCompile(`^\s*(\d+)\s*$`)
	reFloatOnly = regexp.MustCompile(`^\s*([0-9.]+)\s*$`)
)

// IntOrNone converts v to int64, returning nil when not convertible.
func IntOrNone(v any) *int64 {
	switch x := v.(type) {
	case nil:
		return nil
	case int:
		return int64Ptr(int64(x))
	case int64:
		return int64Ptr(x)
	case float64:
		if x != math.Trunc(x) {
			return nil
		}
		return int64Ptr(int64(x))
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return nil
		}
		return &i
	case string:
		if m := reIntOnly.FindStringSubmatch(x); m != nil {
			i, _ := strconv.ParseInt(m[1], 10, 64)
			return &i
		}
	}
	return nil
}

// FloatOrNone converts v to float64, returning nil when not convertible.
func FloatOrNone(v any) *float64 {
	switch x := v.(type) {
	case nil:
		return nil
	case int:
		return float64Ptr(float64(x))
	case int64:
		return float64Ptr(float64(x))
	case float64:
		return &x
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return nil
		}
		return &f
	case string:
		if m := reFloatOnly.FindStringSubmatch(x); m != nil {
			f, _ := strconv.ParseFloat(m[1], 64)
			return &f
		}
	}
	return nil
}

// StrOrNone converts v to a string, returning nil when not convertible.
func StrOrNone(v any) *string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		s := x
		return &s
	case json.Number:
		s := x.String()
		return &s
	case int:
		s := strconv.Itoa(x)
		return &s
	case float64:
		s := strconv.FormatFloat(x, 'f', -1, 64)
		return &s
	case int64:
		s := strconv.FormatInt(x, 10)
		return &s
	}
	return nil
}

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }

// Str returns the string or "".
func Str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "True"
		}
		return "False"
	}
	return ""
}

// Int64 returns the int64 value of v if possible.
func Int64(v any) (int64, bool) {
	p := IntOrNone(v)
	if p == nil {
		return 0, false
	}
	return *p, true
}

// Float64 returns the float64 value of v if possible.
func Float64(v any) (float64, bool) {
	p := FloatOrNone(v)
	if p == nil {
		return 0, false
	}
	return *p, true
}

// ---------------------------------------------------------------------------
// Traversal helpers (minimal traverse_obj)
// ---------------------------------------------------------------------------

// Traverse fetches a nested value from a JSON-like tree using a dotted path
// such as "a.b.c". Missing keys yield nil.
func Traverse(v any, path string) any {
	if path == "" {
		return v
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

// TraverseStr fetches a nested string value.
func TraverseStr(v any, path string) string {
	return Str(Traverse(v, path))
}

// TraverseMap fetches a nested map.
func TraverseMap(v any, path string) map[string]any {
	m, _ := Traverse(v, path).(map[string]any)
	return m
}

// TraverseList fetches a nested slice.
func TraverseList(v any, path string) []any {
	l, _ := Traverse(v, path).([]any)
	return l
}

// ---------------------------------------------------------------------------
// Parsing helpers
// ---------------------------------------------------------------------------

var (
	reDurationClock = regexp.MustCompile(`^(?:(?:(?:(\d+):)?(\d+):)?(\d+):)?(\d{1,2})(?:[.:](\d+))?Z?$`)
	reDurationHuman = regexp.MustCompile(`(?i)P?(?:(\d+)\s*y(?:ears?)?,?\s*)?(?:(\d+)\s*m(?:onths?)?,?\s*)?(?:(\d+)\s*w(?:eeks?)?,?\s*)?(?:(\d+)\s*d(?:ays?)?,?\s*)?T?(?:(\d+)\s*h(?:(?:ou)?rs?)?,?\s*)?(?:(\d+)\s*m(?:in(?:ute)?s?)?,?\s*)?(?:(\d+)(?:\.\d+)?\s*s(?:ec(?:ond)?s?)?)?Z?$`)
	reDurationHMS   = regexp.MustCompile(`(?i)^(?:(\d+[.:]\d+)\s*(?:hours?)|(\d+[.:]\d+)\s*(?:mins?\.?|minutes?))Z?$`)
)

// ParseDuration parses yt-dlp duration strings ("1:23:45", "3:45.5", ISO8601).
func ParseDuration(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "Z")
	// plain number: seconds
	if !strings.ContainsAny(s, ":") {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 && !strings.ContainsAny(s, "hms") {
			return &f
		}
	}
	// clock format: [days:]hours:minutes:seconds[.ms]
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		valid := len(parts) >= 2 && len(parts) <= 4
		nums := make([]float64, len(parts))
		for i, p := range parts {
			f, err := strconv.ParseFloat(p, 64)
			if err != nil || f < 0 {
				valid = false
				break
			}
			nums[i] = f
		}
		if valid {
			var total float64
			for _, n := range nums[:len(nums)-1] {
				total = total*60 + n
			}
			total = total*60 + nums[len(nums)-1]
			return &total
		}
	}
	if m := reDurationHuman.FindStringSubmatch(s); m != nil {
		mults := []float64{86400 * 365, 86400 * 30, 86400 * 7, 86400, 3600, 60, 1}
		var total float64
		found := false
		for i, g := range m[1:] {
			if g != "" {
				v, _ := strconv.ParseFloat(g, 64)
				total += v * mults[i]
				found = true
			}
		}
		if found {
			return &total
		}
	}
	if m := reDurationHMS.FindStringSubmatch(s); m != nil {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		v = strings.ReplaceAll(v, ":", ".")
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			if m[1] != "" {
				f *= 60
			}
			return &f
		}
	}
	return nil
}

var reBytes = regexp.MustCompile(`(?i)^\s*([0-9.]+)\s*([kmgtp]?i?b?)\s*$`)

// ParseBytes parses "10M", "1.5MiB", "1024" into bytes.
func ParseBytes(s string) (int64, bool) {
	m := reBytes.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	var mult float64
	switch strings.ToLower(m[2]) {
	case "", "b":
		mult = 1
	case "k", "kb", "kib":
		mult = 1 << 10
	case "m", "mb", "mib":
		mult = 1 << 20
	case "g", "gb", "gib":
		mult = 1 << 30
	case "t", "tb", "tib":
		mult = 1 << 40
	case "p", "pb", "pib":
		mult = 1 << 50
	default:
		return 0, false
	}
	return int64(f * mult), true
}

var reResolution = regexp.MustCompile(`(^|[^a-zA-Z0-9])(\d+)\s*[xX×,]\s*(\d+)([^a-zA-Z0-9]|$)`)
var reResolutionH = regexp.MustCompile(`(^|[^a-zA-Z0-9])(\d+)[pPiI](\d{2,3})?([^a-zA-Z0-9]|$)`)

// ParseResolution extracts width/height from strings like "1920x1080" or "1080p".
func ParseResolution(s string) (width, height int) {
	if m := reResolution.FindStringSubmatch(s); m != nil {
		width, _ = strconv.Atoi(m[2])
		height, _ = strconv.Atoi(m[3])
		return
	}
	if m := reResolutionH.FindStringSubmatch(s); m != nil {
		height, _ = strconv.Atoi(m[2])
	}
	return
}

// ParseCodecs parses a RFC6381 codecs string into vcodec/acodec/dynamic range.
// Returns vcodec, acodec, dynamicRange.
func ParseCodecs(codecs string) (vcodec, acodec, dynRange string) {
	codecs = strings.TrimSpace(codecs)
	codecs = strings.Trim(codecs, ",")
	if codecs == "" {
		return "", "", ""
	}
	var parts []string
	for _, p := range strings.Split(codecs, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	videoCodecs := map[string]bool{
		"avc1": true, "avc2": true, "avc3": true, "avc4": true,
		"vp9": true, "vp8": true, "hev1": true, "hev2": true,
		"h263": true, "h264": true, "mp4v": true, "hvc1": true,
		"av1": true, "theora": true, "dvh1": true, "dvhe": true,
	}
	audioCodecs := map[string]bool{
		"flac": true, "mp4a": true, "opus": true, "vorbis": true,
		"mp3": true, "aac": true, "ac-4": true, "ac-3": true,
		"ec-3": true, "eac3": true, "dtsc": true, "dtse": true,
		"dtsh": true, "dtsl": true,
	}
	for _, full := range parts {
		lowered := full
		if idx := strings.IndexByte(lowered, '.'); idx >= 0 {
			// lower only the family part, as Python does via ^[^.]+
			lowered = strings.ToLower(lowered[:idx]) + lowered[idx:]
		} else {
			lowered = strings.ToLower(lowered)
		}
		base := lowered
		if idx := strings.IndexByte(base, '.'); idx >= 0 {
			base = base[:idx]
		}
		// strip leading zeros in numeric segments (Python: re.sub(r'0+(?=\d)',''))
		cleaned := stripVersionZeros(base)
		switch {
		case videoCodecs[cleaned]:
			if vcodec == "" {
				vcodec = full
				switch cleaned {
				case "dvh1", "dvhe":
					dynRange = "DV"
				case "av1":
					sub := strings.Split(lowered, ".")
					if len(sub) > 3 && sub[3] == "10" {
						dynRange = "HDR10"
					}
				case "vp9":
					sub := strings.Split(lowered, ".")
					if len(sub) > 1 && strings.TrimLeft(sub[1], "0") == "9" && strings.HasPrefix(lowered, "vp9.2") {
						dynRange = "HDR10"
					}
					if len(sub) > 1 && sub[1] == "2" {
						dynRange = "HDR10"
					}
				}
			}
		case audioCodecs[cleaned]:
			if acodec == "" {
				acodec = full
			}
		case cleaned == "stpp" || cleaned == "wvtt":
			// subtitle codec, ignored
		}
	}
	if vcodec != "" || acodec != "" {
		if vcodec == "" {
			vcodec = "none"
		}
		if acodec == "" {
			acodec = "none"
		}
		return vcodec, acodec, dynRange
	}
	if len(parts) == 2 {
		return parts[0], parts[1], ""
	}
	return "", "", ""
}

func stripVersionZeros(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '0' && i+1 < len(s) && isDigit(s[i+1]) {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

var (
	reKnownExt   = regexp.MustCompile(`(?i)\.(avif|webm|flv|f4v|f4a|f4b|mov|m4v|mp4|m4a|m4b|3gp|3g2|flac|mka|aiff?|au|m3u8|mpd|ts|wmv|avi|mp3|m4v|m4b|ogg|ogv|wav|aac|m3u|nsv|ra|rm|rmvb|amf|swf|wma|vob|dirac|roq|ivr|fli|gl|yuv|dv|vob|tga|bmp)$`)
	reExtFromURL = regexp.MustCompile(`(?i)\.(avif|webm|flv|f4v|f4a|f4b|mov|m4v|mp4|m4a|m4b|3gp|3g2|flac|mka|aiff?|au|ts|wmv|avi|mp3|ogg|ogv|wav|aac|webm|mkv|mpeg|mpg)(?:$|[?#])`)
)

// DetermineExt returns the extension of a URL/name.
func DetermineExt(urlOrName, def string) string {
	if def == "" {
		def = "unknown_video"
	}
	// strip query/fragment
	u := urlOrName
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.LastIndexByte(u, '\\'); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.LastIndexByte(u, '.'); i >= 0 && i+1 < len(u) {
		ext := strings.ToLower(u[i+1:])
		if reKnownExt.MatchString("." + ext) {
			return ext
		}
	}
	return def
}

// mimeExt mirrors yt-dlp's utils.mimetype2ext MAP (keys are either the full
// mimetype or a subtype, looked up in that order).
var mimeExt = map[string]string{
	// video
	"3gpp": "3gp", "mp2t": "ts", "mp4": "mp4", "mpeg": "mpeg",
	"mpegurl": "m3u8", "quicktime": "mov", "webm": "webm", "vp9": "vp9",
	"video/ogg": "ogv", "x-flv": "flv", "x-m4v": "m4v", "x-matroska": "mkv",
	"x-mng": "mng", "x-mp4-fragmented": "mp4", "x-ms-asf": "asf",
	"x-ms-wmv": "wmv", "x-msvideo": "avi", "vnd.dlna.mpeg-tts": "mpeg",
	// application (streaming playlists)
	"dash+xml": "mpd", "f4m+xml": "f4m", "hds+xml": "f4m",
	"vnd.apple.mpegurl": "m3u8", "vnd.ms-sstr+xml": "ism", "x-mpegurl": "m3u8",
	// audio
	"audio/mp4": "m4a", "audio/mpeg": "mp3", "audio/webm": "webm",
	"audio/x-matroska": "mka", "audio/x-mpegurl": "m3u", "aacp": "aac",
	"flac": "flac", "midi": "mid", "ogg": "ogg", "wav": "wav", "wave": "wav",
	"x-aac": "aac", "x-flac": "flac", "x-m4a": "m4a", "x-realaudio": "ra", "x-wav": "wav",
	// image
	"avif": "avif", "bmp": "bmp", "gif": "gif", "jpeg": "jpg", "png": "png",
	"svg+xml": "svg", "tiff": "tif", "vnd.wap.wbmp": "wbmp", "webp": "webp",
	"x-icon": "ico", "x-jng": "jng", "x-ms-bmp": "bmp",
	// caption
	"filmstrip+json": "fs", "smptett+xml": "tt", "ttaf+xml": "dfxp",
	"ttml+xml": "ttml", "x-ms-sami": "sami", "x-subrip": "srt", "x-srt": "srt",
	// misc
	"gzip": "gz", "json": "json", "xml": "xml", "zip": "zip",
}

// Mimetype2ext maps a MIME type to a file extension, mirroring yt-dlp's
// utils.mimetype2ext: the subtype (or full mimetype) is looked up in the MAP,
// then the part after the last '+' is tried, and finally the subtype is
// returned with '+' replaced by '.'.
func Mimetype2ext(mt string) string {
	if mt == "" {
		return ""
	}
	mimetype := strings.ToLower(strings.TrimSpace(strings.SplitN(mt, ";", 2)[0]))
	subtype := mimetype
	if i := strings.LastIndexByte(mimetype, '/'); i >= 0 {
		subtype = mimetype[i+1:]
	}
	for _, key := range []string{mimetype, subtype, lastPlusSegment(subtype)} {
		if key == "" {
			continue
		}
		if ext, ok := mimeExt[key]; ok {
			return ext
		}
	}
	return strings.ReplaceAll(subtype, "+", ".")
}

// lastPlusSegment returns the segment after the last '+'.
func lastPlusSegment(s string) string {
	if i := strings.LastIndexByte(s, '+'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// DetermineProtocol mirrors utils.determine_protocol.
func DetermineProtocol(format map[string]any) string {
	if p, _ := format["protocol"].(string); p != "" {
		return p
	}
	u, _ := format["url"].(string)
	switch {
	case strings.HasPrefix(u, "rtmp"):
		return "rtmp"
	case strings.Contains(u, ".m3u8"):
		return "m3u8_native"
	case strings.Contains(u, ".f4m"):
		return "f4m"
	}
	if i := strings.Index(u, "://"); i >= 0 {
		return u[:i]
	}
	return "https"
}

// ---------------------------------------------------------------------------
// Filenames
// ---------------------------------------------------------------------------

// SanitizeFilename removes/replaces characters that are unsafe in filenames.
// Port of utils.sanitize_filename (default is_id/unrestricted mode).
func SanitizeFilename(s string) string {
	return sanitizeFilename(s, false, false)
}

// SanitizeFilenameRestricted ports sanitize_filename(restricted=True),
// used by --restrict-filenames.
func SanitizeFilenameRestricted(s string) string {
	return sanitizeFilename(s, true, false)
}

// SanitizeFilenameID ports sanitize_filename(is_id=True): keeps IDs mostly intact.
func SanitizeFilenameID(s string) string {
	return sanitizeFilename(s, false, true)
}

var reTimestampInName = regexp.MustCompile(`[0-9]+(?::[0-9]+)+`)

func sanitizeFilename(s string, restricted, isID bool) string {
	if s == "" {
		return ""
	}
	if restricted {
		s = stripDiacritics(s)
	}
	s = reTimestampInName.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(m, ":", "_")
	})
	var b strings.Builder
	for _, r := range s {
		switch {
		case !restricted && r == '\n':
			b.WriteString("\x00 ")
		case !isID && !restricted && strings.ContainsRune(`"*:<>?|/\`, r):
			switch r {
			case '/':
				b.WriteRune(0x29F8)
			case '\\':
				b.WriteRune(0x29F9)
			default:
				b.WriteRune(r + 0xfee0)
			}
		case r == '?' || r < 32 || r == 127:
			// dropped
		case r == '"':
			if !restricted {
				b.WriteString("'")
			}
		case r == ':':
			if restricted {
				b.WriteString("\x00_\x00-")
			} else {
				b.WriteString("\x00 \x00-")
			}
		case strings.ContainsRune(`\/|*<>`, r):
			b.WriteString("\x00_")
		case restricted && (strings.ContainsRune(`!&'()[]{}$;`+"`"+`,^#`, r) || unicode.IsSpace(r) || r > 127):
			b.WriteString("\x00_")
		default:
			b.WriteRune(r)
		}
	}
	result := b.String()
	if !isID {
		result = collapseMarkerPairs(result)
		result = stripMarkers(result)
	}
	result = strings.ReplaceAll(result, "\x00", "")
	if result == "" {
		return "_"
	}
	return result
}

// collapseMarkerPairs removes runs of identical \0X pairs.
func collapseMarkerPairs(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0 && i+1 < len(s) {
			pair := s[i : i+2]
			b.WriteString(pair)
			i += 2
			for i+1 < len(s) && s[i] == 0 && s[i:i+2] == pair {
				i += 2
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// stripMarkers strips marker pairs (and adjacent [ _-]) from both ends,
// mirroring the anchored regexes of the Python original.
func stripMarkers(s string) string {
	// start
	if len(s) >= 2 && s[0] == 0 {
		s = s[2:]
		for {
			moved := false
			if len(s) >= 2 && s[0] == 0 {
				s = s[2:]
				moved = true
			}
			for len(s) > 0 && (s[0] == ' ' || s[0] == '_' || s[0] == '-') {
				s = s[1:]
				moved = true
			}
			if !moved {
				break
			}
		}
	}
	// end
	if len(s) >= 2 && s[len(s)-2] == 0 {
		s = s[:len(s)-2]
		for {
			moved := false
			for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '_' || s[len(s)-1] == '-') {
				s = s[:len(s)-1]
				moved = true
			}
			if len(s) >= 2 && s[len(s)-2] == 0 {
				s = s[:len(s)-2]
				moved = true
			}
			if !moved {
				break
			}
		}
	}
	return s
}

var reCombining = regexp.MustCompile(`\p{Mn}`)

// stripDiacritics approximates NFKD + accent table lookup: decompose and drop
// combining marks, keeping base letters.
func stripDiacritics(s string) string {
	// NFD-like decomposition for common Latin accents without external deps:
	var b strings.Builder
	for _, r := range s {
		if d := nfdMap[r]; d != "" {
			b.WriteString(d)
			continue
		}
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// nfdMap covers accented letters that need manual decomposition.
var nfdMap = map[rune]string{
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u",
	'Ñ': "N", 'ñ': "n", 'Ç': "C", 'ç': "c",
	'Ý': "Y", 'ý': "y", 'ÿ': "y",
	'Æ': "AE", 'æ': "ae", 'Œ': "OE", 'œ': "oe", 'ß': "ss",
	'Ð': "D", 'ð': "d", 'Þ': "Th", 'þ': "th",
}

// SanitizePath normalizes a path (replaces invalid segments with #).
func SanitizePath(p string) string {
	if p == "" {
		return p
	}
	root := ""
	if strings.HasPrefix(p, "/") {
		root = "/"
	}
	parts := strings.Split(p, "/")
	var out []string
	invalid := regexp.MustCompile(`[/<>:"\|\\?\*]|[\s.]$`)
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[:len(out)-1]
			} else {
				out = append(out, "..")
			}
			continue
		}
		out = append(out, invalid.ReplaceAllString(part, "#"))
	}
	res := root + strings.Join(out, "/")
	if res == "" {
		return "."
	}
	return res
}

// SanitizeURL makes sure the URL has a scheme.
func SanitizeURL(u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	if i := strings.Index(u, "://"); i < 0 || strings.ContainsAny(u[:i], "./\\ ") {
		// likely missing scheme
		if !strings.Contains(u, ":") {
			return "http://" + u
		}
	}
	return u
}

// ---------------------------------------------------------------------------
// HTML helpers (regex-based, sufficient for extraction use-cases)
// ---------------------------------------------------------------------------

var (
	reHTMLUnescape = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
		"&#x27;", "'", "&#39;", "'", "&apos;", "'", "&nbsp;", " ",
		"&#x3D;", "=", "&#61;", "=", "&#x2F;", "/", "&#47;", "/",
	)
	reStyleStrip      = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	reScriptStrip     = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	reTagAll          = regexp.MustCompile(`<[^>]+>`)
	reCleanHTMLSpaces = regexp.MustCompile(`\s+`)
)

// UnescapeHTML decodes common HTML entities.
func UnescapeHTML(s string) string {
	s = reHTMLEntityNum.ReplaceAllStringFunc(s, func(m string) string {
		num := m[2 : len(m)-1]
		var r rune
		var err error
		if strings.HasPrefix(strings.ToLower(m[2:4]), "x") {
			var v int64
			v, err = strconv.ParseInt(num[1:], 16, 32)
			r = rune(v)
		} else {
			var v int64
			v, err = strconv.ParseInt(num, 10, 32)
			r = rune(v)
		}
		if err != nil || r == 0 {
			return m
		}
		return string(r)
	})
	return reHTMLUnescape.Replace(s)
}

var reHTMLEntityNum = regexp.MustCompile(`&#(x?[0-9a-fA-F]+);`)

// CleanHTML strips tags and unescapes entities.
func CleanHTML(s string) string {
	s = reStyleStrip.ReplaceAllString(s, " ")
	s = reScriptStrip.ReplaceAllString(s, " ")
	s = reTagAll.ReplaceAllString(s, " ")
	s = UnescapeHTML(s)
	s = reCleanHTMLSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

// OrderedSet deduplicates while preserving order.
func OrderedSet(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	return out
}

// DetermineHostname returns the host of a URL.
func DetermineHostname(rawurl string) string {
	u, err := url.Parse(rawurl)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// URLJoin resolves ref against base.
func URLJoin(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// FormatDurationString renders seconds as H:MM:SS (yt-dlp duration_string).
func FormatDurationString(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	total := int(math.Round(secs))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatSeconds mirrors utils.formatSeconds (used by --get-duration etc).
func FormatSeconds(secs float64) string {
	return FormatDurationString(secs)
}

// FormatBytes renders a byte count in binary units, e.g. "1.96MiB".
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	units := "KMGTPE"
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%ciB", float64(n)/float64(div), units[exp])
}

// FormatClock renders a duration as MM:SS or HH:MM:SS.
func FormatClock(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 0 {
		secs = 0
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// UnixToYYYYMMDD formats a unix timestamp as yt-dlp's upload_date.
func UnixToYYYYMMDD(ts int64) string {
	return time.Unix(ts, 0).UTC().Format("20060102")
}

// ParseYYYYMMDD parses yt-dlp date options (YYYYMMDD, YYYY-MM-DD, YYYY, ...).
func ParseYYYYMMDD(s string) (year, month, day int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, fmt.Errorf("empty date")
	}
	reFull := regexp.MustCompile(`^(\d{4})-?(\d{2})?-?(\d{2})?$`)
	m := reFull.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("invalid date %q", s)
	}
	year, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		month, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		day, _ = strconv.Atoi(m[3])
	}
	return year, month, day, nil
}

// ExpandPath expands ~ and environment variables in a path.
func ExpandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	return os.ExpandEnv(p)
}

// IsHTML reports whether the given bytes look like an HTML document.
func IsHTML(firstBytes []byte) bool {
	s := strings.ToLower(strings.TrimSpace(string(firstBytes)))
	switch {
	case s == "", strings.Contains(s, "<!--"):
		return true
	case strings.HasPrefix(s, "<?xml"), strings.HasPrefix(s, "<!doctype html"),
		strings.HasPrefix(s, "<html"), strings.HasPrefix(s, "<head"),
		strings.HasPrefix(s, "<iframe"), strings.HasPrefix(s, "<div"),
		strings.HasPrefix(s, "<span"), strings.HasPrefix(s, "<video"),
		strings.HasPrefix(s, "<!doctype"), strings.HasPrefix(s, "<script"),
		strings.HasPrefix(s, "<a "), strings.HasPrefix(s, "<meta"),
		strings.HasPrefix(s, "<title"), strings.HasPrefix(s, "<body"):
		return true
	}
	return false
}

// Capitalize capitalizes the first letter.
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// JoinPath joins path elements like os.path.join.
func JoinPath(elem ...string) string {
	return path.Join(elem...)
}
