package utils

import "testing"

func TestSanitizeFilenameDefault(t *testing.T) {
	cases := map[string]string{
		"  hello  ":   "  hello  ",
		"a_b__c":      "a_b__c",
		".hidden":     ".hidden",
		"-lead":       "-lead",
		"foo/bar:baz": "foo⧸bar：baz",
		"1:23:45 x":   "1_23_45 x",
		`q"uery?*<>|`: "q＂uery？＊＜＞｜",
		"__init__":    "__init__",
		"test.mp4 ":   "test.mp4 ",
		"  ":          "  ",
		"Ünïcödé":     "Ünïcödé",
		"":            "",
	}
	for in, want := range cases {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFilenameRestricted(t *testing.T) {
	cases := map[string]string{
		"  hello  ":   "hello",
		"a_b__c":      "a_b__c",
		".hidden":     ".hidden",
		"-lead":       "-lead",
		"foo/bar:baz": "foo_bar_-baz",
		"1:23:45 x":   "1_23_45_x",
		`q"uery?*<>|`: "query",
		"__init__":    "__init__",
		"test.mp4 ":   "test.mp4",
		"  ":          "_",
		"Ünïcödé":     "Unicode",
	}
	for in, want := range cases {
		if got := SanitizeFilenameRestricted(in); got != want {
			t.Errorf("SanitizeFilenameRestricted(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]float64{
		"1:23:45": 5025,
		"3:45.5":  225.5,
		"45":      45,
		"1:02:03": 3723,
	}
	for in, want := range cases {
		got := ParseDuration(in)
		if got == nil || *got != want {
			t.Errorf("ParseDuration(%q) = %v, want %v", in, got, want)
		}
	}
	for _, s := range []string{"", "  ", "abc"} {
		if ParseDuration(s) != nil {
			t.Errorf("ParseDuration(%q) should be nil", s)
		}
	}
}

func TestParseBytes(t *testing.T) {
	cases := map[string]int64{
		"1024":   1024,
		"10M":    10 << 20,
		"1.5MiB": int64(1.5 * (1 << 20)),
		"2G":     2 << 30,
	}
	for in, want := range cases {
		got, ok := ParseBytes(in)
		if !ok || got != want {
			t.Errorf("ParseBytes(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
}

func TestParseCodecs(t *testing.T) {
	v, a, d := ParseCodecs("avc1.640028, mp4a.40.2")
	if v != "avc1.640028" || a != "mp4a.40.2" || d != "" {
		t.Errorf("got %q %q %q", v, a, d)
	}
	v, a, d = ParseCodecs("vp09.00.10.08")
	if v != "vp09.00.10.08" || a != "none" || d != "" {
		t.Errorf("got %q %q %q", v, a, d)
	}
	v, a, d = ParseCodecs("dvh1.08.07")
	if v != "dvh1.08.07" || a != "none" || d != "DV" {
		t.Errorf("got %q %q %q", v, a, d)
	}
	v, a, d = ParseCodecs("av01.0.08M.08.0.110.01.01.01.0")
	if v == "" || a != "none" {
		t.Errorf("got %q %q", v, a)
	}
}

func TestDetermineExt(t *testing.T) {
	if got := DetermineExt("https://x.com/v.mp4?a=b", ""); got != "mp4" {
		t.Errorf("got %q", got)
	}
	if got := DetermineExt("https://x.com/v", "unknown_video"); got != "unknown_video" {
		t.Errorf("got %q", got)
	}
}

func TestMimetype2ext(t *testing.T) {
	cases := map[string]string{
		"audio/webm":                    "webm",
		"audio/webm; codecs=opus":       "webm",
		"audio/mp4":                     "m4a",
		"audio/mp4; codecs=mp4a":        "m4a",
		"audio/mpeg":                    "mp3",
		"audio/ogg":                     "ogg",
		"audio/opus":                    "opus",
		"video/mp4":                     "mp4",
		"video/webm":                    "webm",
		"video/quicktime":               "mov",
		"video/x-matroska":              "mkv",
		"application/vnd.apple.mpegurl": "m3u8",
		"application/x-mpegurl":         "m3u8",
		"application/dash+xml":          "mpd",
		"application/ttml+xml":          "ttml",
		"image/jpeg":                    "jpg",
		"image/png":                     "png",
	}
	for mt, want := range cases {
		if got := Mimetype2ext(mt); got != want {
			t.Errorf("Mimetype2ext(%q) = %q, want %q", mt, got, want)
		}
	}
	if got := Mimetype2ext(""); got != "" {
		t.Errorf("Mimetype2ext(\"\") = %q, want empty", got)
	}
	if got := Mimetype2ext("application/x-foo"); got != "x-foo" {
		t.Errorf("Mimetype2ext fallback = %q, want %q", got, "x-foo")
	}
}
