package core

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func TestGetCompatibleExt(t *testing.T) {
	cases := []struct {
		name string
		in   []infodict.Format
		want string
	}{
		{
			name: "av1 + opus -> webm",
			in: []infodict.Format{
				{VCodec: "av01.0.00M.08", ACodec: "none", Ext: "mp4"},
				{VCodec: "none", ACodec: "opus", Ext: "webm"},
			},
			want: "webm",
		},
		{
			name: "avc1 + mp4a -> mp4",
			in: []infodict.Format{
				{VCodec: "avc1.4D400C", ACodec: "none", Ext: "mp4"},
				{VCodec: "none", ACodec: "mp4a.40.2", Ext: "m4a"},
			},
			want: "mp4",
		},
		{
			name: "vp9 + opus -> webm",
			in: []infodict.Format{
				{VCodec: "vp9", ACodec: "none", Ext: "webm"},
				{VCodec: "none", ACodec: "opus", Ext: "webm"},
			},
			want: "webm",
		},
		{
			name: "vp9 + mp4a falls back to mkv",
			in: []infodict.Format{
				{VCodec: "vp9", ACodec: "none", Ext: "webm"},
				{VCodec: "none", ACodec: "mp4a.40.2", Ext: "m4a"},
			},
			want: "mkv",
		},
	}
	for _, c := range cases {
		if got := getCompatibleExt(c.in); got != c.want {
			t.Errorf("%s: getCompatibleExt = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSanitizeCodec(t *testing.T) {
	if got := sanitizeCodec([]string{"av01.0.00M.08"}); got != "av1" {
		t.Errorf("sanitizeCodec = %q, want av1", got)
	}
	if got := sanitizeCodec(nil); got != "" {
		t.Errorf("sanitizeCodec(nil) = %q, want empty", got)
	}
}

func TestSplitFormatFilters(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"ext=mp4", []string{"ext=mp4"}},
		{"height<=240,vcodec*=avc", []string{"height<=240", "vcodec*=avc"}},
		{`vcodec~="a,b",ext=mp4`, []string{`vcodec~="a,b"`, "ext=mp4"}},
		{"vcodec^=avc1", []string{"vcodec^=avc1"}},
	}
	for _, c := range cases {
		got := splitFormatFilters(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitFormatFilters(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitFormatFilters(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestFormatFilters(t *testing.T) {
	video := infodict.Format{FormatID: "133", VCodec: "avc1.4D400C", ACodec: "none", Ext: "mp4", Height: 240, TBR: 183, FormatNote: "240p", Filesize: 200 * 1024}
	audio := infodict.Format{FormatID: "251", VCodec: "none", ACodec: "opus", Ext: "webm", ABR: 106}

	cases := []struct {
		spec    string
		f       infodict.Format
		want    bool
		wantErr bool
	}{
		{"vcodec^=avc1", video, true, false},
		{"vcodec^=avc1", audio, false, false},
		{"vcodec*=avc", video, true, false},
		{"vcodec$=4D400C", video, true, false},
		{"vcodec=avc1.4D400C", video, true, false},
		{"vcodec=avc1", video, false, false},
		{"vcodec!=av01", video, true, false},
		{`vcodec~="^avc1"`, video, true, false},
		{"ext=mp4", video, true, false},
		{"ext=mp4", audio, false, false},
		{"acodec=opus", audio, true, false},
		{"height<=144", video, false, false},
		{"height<=240", video, true, false},
		{"height>=240", video, true, false},
		{"format_id=133", video, false, false}, // numeric value vs string field
		{"format_id^=13", video, true, false},
		{"filesize<250K", video, true, false},
		{"filesize<50K", video, false, false},
		{"filesize<?250K", audio, true, false}, // field absent -> none-inclusive
		{"filesize<250K", audio, false, false},
		{"vcodec=av01", video, false, false},
		{"!vcodec=av01", video, false, true}, // invalid: negation must follow a key
		{"nonsense~=(", video, false, true},
	}
	for _, c := range cases {
		fn, err := buildFormatFilter(c.spec)
		if c.wantErr {
			if err == nil {
				t.Errorf("buildFormatFilter(%q): expected error", c.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("buildFormatFilter(%q): unexpected error: %v", c.spec, err)
			continue
		}
		if got := fn(c.f); got != c.want {
			t.Errorf("filter %q on %s = %v, want %v", c.spec, c.f.FormatID, got, c.want)
		}
	}
}

func TestSelectFormatsInvalidFilter(t *testing.T) {
	y := &YoutubeDL{}
	formats := []infodict.Format{{FormatID: "1", VCodec: "avc1", ACodec: "none", Ext: "mp4"}}
	if _, err := y.selectFormats(formats, "bestvideo[!!broken!!]"); err == nil {
		t.Error("expected error for invalid filter spec")
	}
	if got, err := y.selectFormats(formats, "bestvideo[vcodec^=avc]"); err != nil || len(got) != 1 {
		t.Errorf("valid filter: got %d formats, err %v", len(got), err)
	}
}

func TestSelectBestWorstFallbackGated(t *testing.T) {
	y := &YoutubeDL{}
	video := infodict.Format{FormatID: "269", VCodec: "avc1.4D400B", ACodec: "none", Ext: "mp4"}
	videoHi := infodict.Format{FormatID: "399", VCodec: "av01.0.00M.08", ACodec: "none", Ext: "mp4"}
	audio := infodict.Format{FormatID: "251", VCodec: "none", ACodec: "opus", Ext: "webm"}

	// YouTube-like: video-only + audio-only but no combined format.
	// incomplete_formats is False, so best/worst must NOT fall back.
	single := []infodict.Format{video, videoHi, audio}
	cases := map[string][]string{
		"best":          nil,
		"worst":         nil,
		"b":             nil,
		"best[ext=mp4]": nil,
	}
	for spec, want := range cases {
		got, _ := y.selectFormats(single, spec)
		if !equalStrings(fmtIDs(got), want) {
			t.Errorf("%s on single-stream formats = %v, want %v", spec, fmtIDs(got), want)
		}
	}

	// SoundCloud-like: all formats audio-only -> incomplete_formats True -> best
	// falls back to the best audio-only format.
	audioOnly := []infodict.Format{
		{FormatID: "1", VCodec: "none", ACodec: "mp4a.40.2", Ext: "m4a"},
		{FormatID: "2", VCodec: "none", ACodec: "opus", Ext: "webm"},
	}
	if got, _ := y.selectFormats(audioOnly, "best"); !equalStrings(fmtIDs(got), []string{"2"}) {
		t.Errorf("best on audio-only = %v, want [2]", fmtIDs(got))
	}
	if got, _ := y.selectFormats(audioOnly, "worst"); !equalStrings(fmtIDs(got), []string{"1"}) {
		t.Errorf("worst on audio-only = %v, want [1]", fmtIDs(got))
	}
}

func TestSelectVideoExtSeparateFallback(t *testing.T) {
	y := &YoutubeDL{}
	videoMP4 := infodict.Format{FormatID: "269", VCodec: "avc1.4D400B", ACodec: "none", Ext: "mp4"}
	mergedWebm := infodict.Format{FormatID: "22", VCodec: "vp9", ACodec: "opus", Ext: "webm"}
	mergedMP4 := infodict.Format{FormatID: "18", VCodec: "avc1", ACodec: "mp4a.40.2", Ext: "mp4"}

	// A merged (non-mp4) format exists -> no fallback to video-only mp4.
	withMerged := []infodict.Format{videoMP4, mergedWebm}
	if got, _ := y.selectFormats(withMerged, "mp4"); len(got) != 0 {
		t.Errorf("mp4 with a merged webm = %v, want empty", fmtIDs(got))
	}
	// A merged mp4 format exists -> selected directly.
	withMergedMP4 := []infodict.Format{videoMP4, mergedMP4}
	if got, _ := y.selectFormats(withMergedMP4, "mp4"); !equalStrings(fmtIDs(got), []string{"18"}) {
		t.Errorf("mp4 with merged mp4 = %v, want [18]", fmtIDs(got))
	}
	// No merged format at all -> fall back to the video-only mp4.
	noMerged := []infodict.Format{videoMP4, {FormatID: "251", VCodec: "none", ACodec: "opus", Ext: "webm"}}
	if got, _ := y.selectFormats(noMerged, "mp4"); !equalStrings(fmtIDs(got), []string{"269"}) {
		t.Errorf("mp4 without merged format = %v, want [269]", fmtIDs(got))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fmtIDs(got []infodict.Format) []string {
	if len(got) == 0 {
		return nil
	}
	out := make([]string, len(got))
	for i := range got {
		out[i] = got[i].FormatID
	}
	return out
}
