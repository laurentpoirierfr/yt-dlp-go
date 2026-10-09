package utils

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FilesizeFromTbr estimates a filesize from a total bitrate (kbps) and duration.
func FilesizeFromTbr(tbr *float64, duration *float64) *int64 {
	if tbr == nil || duration == nil || *tbr == 0 || *duration == 0 {
		return nil
	}
	v := int64(*duration * *tbr * (1000 / 8))
	return &v
}

// StrToInt is str_to_int: strips , . + then parses.
func StrToInt(s string) *int64 {
	re := regexp.MustCompile(`[,.+]`)
	s = re.ReplaceAllString(s, "")
	return IntOrNone(s)
}

var reCountPrefix = regexp.MustCompile(`^[^\d]+\s`)
var reCountPure = regexp.MustCompile(`^[\d,.]+$`)
var reCountNum = regexp.MustCompile(`([\d,.]+)(?:$|\s)`)

// ParseCount parses human counts ("1.2M views", "1,234", "45k").
func ParseCount(s string) *int64 {
	if s == "" {
		return nil
	}
	s = strings.TrimSpace(reCountPrefix.ReplaceAllString(s, ""))
	if reCountPure.MatchString(s) {
		return StrToInt(s)
	}
	units := map[string]int64{
		"k": 1000, "K": 1000,
		"m": 1000 * 1000, "M": 1000 * 1000,
		"kk": 1000 * 1000, "KK": 1000 * 1000,
		"b": 1000 * 1000 * 1000, "B": 1000 * 1000 * 1000,
	}
	for _, tok := range strings.Fields(s) {
		if v, ok := units[tok]; ok {
			return &v
		}
	}
	if m := reCountNum.FindStringSubmatch(s); m != nil {
		return StrToInt(m[1])
	}
	return nil
}

var reISO8601 = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})(?:T| )(\d{2}:\d{2}:\d{2})`)

// ParseISO8601 parses an ISO8601 datetime to a unix timestamp (seconds).
// When the input carries no timezone offset, UTC is assumed (yt-dlp's
// parse_iso8601 with an explicit default timezone).
func ParseISO8601(s string) *int64 {
	if s == "" {
		return nil
	}
	m := reISO8601.FindStringSubmatchIndex(s)
	if m == nil {
		return nil
	}
	loc := time.UTC
	rest := s[m[5]:] // after HH:MM:SS
	if len(rest) >= 6 && (rest[0] == '+' || rest[0] == '-') && isDigit(rest[1]) && isDigit(rest[2]) {
		h, _ := strconv.Atoi(rest[1:3])
		mm, _ := strconv.Atoi(rest[4:6])
		dur := time.Duration(h)*time.Hour + time.Duration(mm)*time.Minute
		if rest[0] == '-' {
			dur = -dur
		}
		loc = time.FixedZone("", int(dur.Seconds()))
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05",
		s[m[2]:m[3]]+"T"+s[m[4]:m[5]], loc)
	if err != nil {
		return nil
	}
	ts := t.Unix()
	return &ts
}

// ISO8601ToDate converts an ISO8601 date (YYYY-MM-DD) for date fields.
func ISO8601ToDate(s string) string {
	if len(s) >= 10 {
		return strings.ReplaceAll(s[:10], "-", "")
	}
	return ""
}

// SafeInt converts any numeric to int (0 on failure).
func SafeInt(v any) int {
	f, ok := Float64(v)
	if !ok {
		return 0
	}
	return int(f)
}
