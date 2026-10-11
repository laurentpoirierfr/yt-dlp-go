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
