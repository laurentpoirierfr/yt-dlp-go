package subtitles

import (
	"reflect"
	"strings"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func subs(exts ...string) []infodict.Subtitle {
	out := make([]infodict.Subtitle, 0, len(exts))
	for _, e := range exts {
		out = append(out, infodict.Subtitle{Ext: e})
	}
	return out
}

func TestSelectLangs(t *testing.T) {
	avail := map[string][]infodict.Subtitle{
		"en":        subs("vtt"),
		"en-US":     subs("vtt"),
		"fr":        subs("vtt"),
		"ja":        subs("vtt"),
		"live_chat": subs("json3"),
	}
	cases := []struct {
		name string
		spec []string
		want []string
	}{
		{"empty means all", nil, []string{"en", "en-US", "fr", "ja", "live_chat"}},
		{"explicit all", []string{"all"}, []string{"en", "en-US", "fr", "ja", "live_chat"}},
		{"exact", []string{"en"}, []string{"en"}},
		{"multi", []string{"fr", "ja"}, []string{"fr", "ja"}},
		{"regex", []string{"en.*"}, []string{"en", "en-US"}},
		{"exclusion", []string{"all", "-live_chat"}, []string{"en", "en-US", "fr", "ja"}},
		{"regex exclusion", []string{"all", "-en.*"}, []string{"fr", "ja", "live_chat"}},
		{"no match", []string{"de"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SelectLangs(avail, c.spec)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("SelectLangs(%v) = %v, want %v", c.spec, got, c.want)
			}
		})
	}
}

func TestChoose(t *testing.T) {
	list := subs("srv1", "srv2", "srv3", "json3", "ttml", "vtt")
	if got, ok := Choose(list, ""); !ok || got.Ext != "vtt" {
		t.Errorf("Choose best = %q,%v, want vtt", got.Ext, ok)
	}
	if got, ok := Choose(list, "json3"); !ok || got.Ext != "json3" {
		t.Errorf("Choose json3 = %q,%v", got.Ext, ok)
	}
	if got, ok := Choose(list, "ass/srt/best"); !ok || got.Ext != "vtt" {
		t.Errorf("Choose ass/srt/best = %q,%v, want vtt fallback", got.Ext, ok)
	}
	if _, ok := Choose(subs("srv1", "srv2"), "srt"); ok {
		t.Error("Choose srt should not match srv formats")
	}
}

func TestConvertNoop(t *testing.T) {
	data, ext, err := Convert([]byte("x"), "vtt", "vtt")
	if err != nil || ext != "vtt" || string(data) != "x" {
		t.Errorf("Convert vtt->vtt = %q,%q,%v", data, ext, err)
	}
	data, ext, err = Convert([]byte("x"), "vtt", "")
	if err != nil || ext != "vtt" || string(data) != "x" {
		t.Errorf("Convert vtt->none = %q,%q,%v", data, ext, err)
	}
}

func TestConvertUnsupported(t *testing.T) {
	if _, _, err := Convert([]byte("x"), "ttml", "srt"); err == nil {
		t.Error("Convert ttml->srt should fail")
	}
}

func TestVTTToSRT(t *testing.T) {
	vtt := "WEBVTT\nKind: captions\n\n00:00:01.000 --> 00:00:03.000\nHello <c>world</c>\n\n00:00:03.500 --> 00:00:04.000 align:start\nSecond line\n"
	got := VTTToSRT(vtt)
	want := "1\n00:00:01,000 --> 00:00:03,000\nHello world\n\n2\n00:00:03,500 --> 00:00:04,000\nSecond line\n\n"
	if got != want {
		t.Errorf("VTTToSRT = %q, want %q", got, want)
	}
}

func TestJSON3ToSRT(t *testing.T) {
	j := `{"events":[
		{"tStartMs":1000,"dDurationMs":2000,"segs":[{"utf8":"Hello "},{"utf8":"world"}]},
		{"segs":[{"utf8":"\n"}]},
		{"tStartMs":3500,"dDurationMs":500,"segs":[{"utf8":"Second"}]}
	]}`
	got := JSON3ToSRT([]byte(j))
	want := "1\n00:00:01,000 --> 00:00:03,000\nHello world\n\n2\n00:00:03,500 --> 00:00:04,000\nSecond\n\n"
	if got != want {
		t.Errorf("JSON3ToSRT = %q, want %q", got, want)
	}
}

func TestJSON3ToVTT(t *testing.T) {
	j := `{"events":[{"tStartMs":0,"dDurationMs":1500,"segs":[{"utf8":"Hi"}]}]}`
	got := JSON3ToVTT([]byte(j))
	if !strings.HasPrefix(got, "WEBVTT\n\n") {
		t.Fatalf("missing WEBVTT header: %q", got)
	}
	if !strings.Contains(got, "00:00:00.000 --> 00:00:01.500\nHi") {
		t.Errorf("cue = %q", got)
	}
}
