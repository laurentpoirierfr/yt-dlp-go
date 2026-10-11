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
