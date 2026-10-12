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
	normal := b.processFormatStream(cloneMap(base), "web", 0, false, newFormatState(""))
	if normal == nil || normal.Quality == nil || *normal.Quality != 3 {
		t.Fatalf("normal quality = %v, want 3", normal)
	}

	drc := cloneMap(base)
	drc["isDrc"] = true
	got := b.processFormatStream(drc, "web", 0, false, newFormatState(""))
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

func TestExtractStoryboards(t *testing.T) {
	spec := "https://i.ytimg.com/sb/VID/storyboard3_L$L/$N.jpg?sqp=abc" +
		"|48#27#100#10#10#0#default#rs$AAA" +
		"|80#45#136#10#10#10000#M$M#rs$BBB" +
		"|160#90#136#5#5#10000#M$M#rs$CCC" +
		"|320#180#136#3#3#10000#M$M#rs$DDD"
	prs := []map[string]any{{
		"storyboards": map[string]any{
			"playerStoryboardSpecRenderer": map[string]any{"spec": spec},
		},
	}}

	got := (&Base{}).extractStoryboards(prs, 1344)
	if len(got) != 4 {
		t.Fatalf("got %d storyboards, want 4", len(got))
	}

	checks := []struct {
		id            string
		url           string
		width, height int
		rows, cols    int
		frags         int
		lastFragDur   float64
	}{
		{"sb0", "https://i.ytimg.com/sb/VID/storyboard3_L3/M$M.jpg?sqp=abc&sigh=rs$DDD", 320, 180, 3, 3, 16, 9.882352941176578},
		{"sb1", "https://i.ytimg.com/sb/VID/storyboard3_L2/M$M.jpg?sqp=abc&sigh=rs$CCC", 160, 90, 5, 5, 6, 108.70588235294122},
		{"sb2", "https://i.ytimg.com/sb/VID/storyboard3_L1/M$M.jpg?sqp=abc&sigh=rs$BBB", 80, 45, 10, 10, 2, 355.76470588235304},
		{"sb3", "https://i.ytimg.com/sb/VID/storyboard3_L0/default.jpg?sqp=abc&sigh=rs$AAA", 48, 27, 10, 10, 1, 1344},
	}
	for i, c := range checks {
		f := got[i]
		if f.FormatID != c.id || f.Ext != "mhtml" || f.Protocol != "mhtml" ||
			f.ACodec != "none" || f.VCodec != "none" || f.FormatNote != "storyboard" {
			t.Errorf("[%d] unexpected format identity: %+v", i, f)
		}
		if f.URL != c.url || f.Width != c.width || f.Height != c.height ||
			f.Rows != c.rows || f.Columns != c.cols {
			t.Errorf("[%d] got url=%q %dx%d rows=%d cols=%d", i, f.URL, f.Width, f.Height, f.Rows, f.Columns)
		}
		if len(f.Fragments) != c.frags {
			t.Errorf("[%d] got %d fragments, want %d", i, len(f.Fragments), c.frags)
		}
		if last := f.Fragments[len(f.Fragments)-1].Duration; last != c.lastFragDur {
			t.Errorf("[%d] last fragment duration = %v, want %v", i, last, c.lastFragDur)
		}
	}

	if got := (&Base{}).extractStoryboards(nil, 10); got != nil {
		t.Errorf("no storyboards: got %v, want nil", got)
	}
	badSpec := "base|only#three#fields"
	got = (&Base{}).extractStoryboards([]map[string]any{{
		"storyboards": map[string]any{
			"playerStoryboardSpecRenderer": map[string]any{"spec": badSpec},
		},
	}}, 10)
	if len(got) != 0 {
		t.Errorf("malformed spec: got %d formats, want 0", len(got))
	}
}
