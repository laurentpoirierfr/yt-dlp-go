package youtube

import "testing"

func TestQualityIndex(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
	}{
		{"", nil},
		{"unknown", nil},
		{"tiny", f(0)},
		{"audio_quality_medium", f(3)},
		{"small", f(5)},
		{"hd1080", f(9)},
		{"highres", f(13)},
	}
	for _, c := range cases {
		got := qualityIndex(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("qualityIndex(%q) = %v, want nil", c.in, *got)
		case c.want != nil && got == nil:
			t.Errorf("qualityIndex(%q) = nil, want %v", c.in, *c.want)
		case c.want != nil && got != nil && *got != *c.want:
			t.Errorf("qualityIndex(%q) = %v, want %v", c.in, *got, *c.want)
		}
	}
}

func TestProcessFormatStreamDrcPenalty(t *testing.T) {
	b := &Base{}
	b.SetDownloader(newHarness())
	base := map[string]any{
		"itag":           "251",
		"mimeType":       `audio/webm; codecs="opus"`,
		"audioQuality":   "AUDIO_QUALITY_MEDIUM",
		"averageBitrate": "107429",
	}
	normal := b.processFormatStream(cloneMap(base), "web", 0, false)
	if normal == nil || normal.Quality == nil || *normal.Quality != 3 {
		t.Fatalf("normal quality = %v, want 3", normal)
	}

	drc := cloneMap(base)
	drc["isDrc"] = true
	got := b.processFormatStream(drc, "web", 0, false)
	if got == nil || got.Quality == nil || *got.Quality != 2.5 {
		t.Fatalf("drc quality = %v, want 2.5", got)
	}
	if got.FormatID != "251-drc" {
		t.Errorf("drc format_id = %q, want 251-drc", got.FormatID)
	}
}

func f(v float64) *float64 { return &v }

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
