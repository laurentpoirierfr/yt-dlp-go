package youtube

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

const testMasterPlaylist = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="233",NAME="Default",DEFAULT=YES,AUTOSELECT=YES,URI="https://rr.example/videoplayback/playlist/itag/233/playlist.m3u8?x=1"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="234",NAME="Default",DEFAULT=YES,AUTOSELECT=YES,URI="https://rr.example/videoplayback/playlist/itag/234/playlist.m3u8?x=1"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="English",DEFAULT=YES,AUTOSELECT=YES,LANGUAGE="en",URI="https://rr.example/timedtext/playlist/en.vtt"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="German",AUTOSELECT=YES,LANGUAGE="de",URI="https://rr.example/timedtext/playlist/de.vtt"
#EXT-X-STREAM-INF:BANDWIDTH=78000,AVERAGE-BANDWIDTH=78000,RESOLUTION=192x144,FRAME-RATE=15,AUDIO="233",CODECS="avc1.4D400B,mp4a.40.2"
https://rr.example/videoplayback/playlist/itag/269/playlist.m3u8?x=1
#EXT-X-STREAM-INF:BANDWIDTH=137995,AVERAGE-BANDWIDTH=137995,RESOLUTION=192x144,FRAME-RATE=15,AUDIO="234",CODECS="vp09.00.11.08.01.05.01.06.00,mp4a.40.2"
https://rr.example/videoplayback/playlist/itag/603/playlist.m3u8?x=1
`

func TestParseM3U8Attributes(t *testing.T) {
	attrs := parseM3U8Attributes(`#EXT-X-STREAM-INF:BANDWIDTH=78000,AVERAGE-BANDWIDTH=78000,RESOLUTION=192x144,FRAME-RATE=15,AUDIO="233",CODECS="avc1.4D400B,mp4a.40.2"`)
	if attrs["BANDWIDTH"] != "78000" {
		t.Errorf("BANDWIDTH = %q, want 78000", attrs["BANDWIDTH"])
	}
	if attrs["AUDIO"] != "233" {
		t.Errorf("AUDIO = %q, want 233", attrs["AUDIO"])
	}
	if attrs["RESOLUTION"] != "192x144" {
		t.Errorf("RESOLUTION = %q, want 192x144", attrs["RESOLUTION"])
	}
}

func TestParseM3U8MasterPlaylist(t *testing.T) {
	formats, subs := parseM3U8FormatsAndSubtitles(testMasterPlaylist,
		"https://rr.example/videoplayback/playlist/playlist.m3u8", "mp4", "m3u8_native")

	if len(subs["en"]) != 1 || subs["en"][0].Ext != "vtt" {
		t.Errorf("en subs = %+v, want 1 vtt", subs["en"])
	}
	if len(subs["de"]) != 1 {
		t.Errorf("de subs missing: %+v", subs["de"])
	}

	audio := map[string]*infodict.Format{}
	video := map[string]*infodict.Format{}
	for i := range formats {
		if formats[i].VCodec == "none" {
			audio[formats[i].FormatID] = &formats[i]
		} else {
			video[formats[i].FormatID] = &formats[i]
		}
	}

	if len(video) != 2 {
		t.Fatalf("video formats = %v, want 2", len(video))
	}
	// format_id is the bitrate-derived fallback until the itag is applied later.
	v, ok := video["78"]
	if !ok {
		t.Fatalf("video format id=78 not found, got %v", keysOf(video))
	}
	if v.Height != 144 || v.Width != 192 {
		t.Errorf("resolution = %dx%d, want 192x144", v.Width, v.Height)
	}
	if v.FPS != 15 {
		t.Errorf("fps = %v, want 15", v.FPS)
	}
	if v.TBR != 78 {
		t.Errorf("tbr = %v, want 78", v.TBR)
	}
	if v.VCodec != "avc1.4D400B" || v.ACodec != "none" {
		t.Errorf("codecs = %q/%q, want avc1.4D400B/none", v.VCodec, v.ACodec)
	}

	if len(audio) != 2 {
		t.Fatalf("audio formats = %v, want 2", len(audio))
	}
	// EXT-X-MEDIA audio entries get quality-tagged source preferences from the
	// video group bitrates: 233 (lower) is low, 234 is high.
	a233, ok := audio["233-Default"]
	if !ok {
		t.Fatalf("audio 233-Default not found, got %v", keysOf(audio))
	}
	if a233.SourcePreference == nil || *a233.SourcePreference != 0 {
		t.Errorf("233 source_preference = %v, want 0", a233.SourcePreference)
	}
	if want := "Default, low"; a233.FormatNote != want {
		t.Errorf("233 format_note = %q, want %q", a233.FormatNote, want)
	}
	a234 := audio["234-Default"]
	if a234.SourcePreference == nil || *a234.SourcePreference != 1 {
		t.Errorf("234 source_preference = %v, want 1", a234.SourcePreference)
	}
	if want := "Default, high"; a234.FormatNote != want {
		t.Errorf("234 format_note = %q, want %q", a234.FormatNote, want)
	}
}

func TestParseM3U8MediaPlaylist(t *testing.T) {
	doc := "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6.000,\nseg-0.ts\n"
	formats, _ := parseM3U8FormatsAndSubtitles(doc, "https://x.example/playlist.m3u8", "mp4", "m3u8_native")
	if len(formats) != 1 {
		t.Fatalf("formats = %v, want 1", len(formats))
	}
	if formats[0].URL != "https://x.example/playlist.m3u8" || formats[0].Ext != "mp4" {
		t.Errorf("media playlist format = %+v", formats[0])
	}
}

func TestProcessManifestFormatQuality(t *testing.T) {
	st := newFormatState("")
	// res_qualities accumulated from the https pass of the reference video.
	st.setResQuality(144, "tiny")
	st.setResQuality(240, "small")
	st.itagQualities["160"] = "tiny"

	// Audio-only itag with no known quality stays at -1.
	audio := &infodict.Format{VCodec: "none"}
	audio.URL = "https://rr.example/playlist/itag/233/playlist.m3u8"
	if !(&Base{}).processManifestFormat(audio, "hls", "web", "233", st) {
		t.Fatal("audio format rejected")
	}
	if audio.FormatID != "233" {
		t.Errorf("format_id = %q, want 233", audio.FormatID)
	}
	if audio.SourcePreference == nil || *audio.SourcePreference != -1 {
		t.Errorf("source_preference = %v, want -1", audio.SourcePreference)
	}
	if audio.Quality == nil || *audio.Quality != -1 {
		t.Errorf("233 quality = %v, want -1", audio.Quality)
	}

	// 144p video resolves quality from the nearest known resolution.
	v144 := &infodict.Format{VCodec: "avc1.4D400B", ACodec: "none", Height: 144}
	v144.URL = "https://rr.example/playlist/itag/269/playlist.m3u8"
	if !(&Base{}).processManifestFormat(v144, "hls", "web", "269", st) {
		t.Fatal("144p format rejected")
	}
	if v144.FormatID != "269" {
		t.Errorf("format_id = %q, want 269", v144.FormatID)
	}
	if v144.Quality == nil || *v144.Quality != 0 {
		t.Errorf("269 quality = %v, want 0", v144.Quality)
	}

	// 240p video maps to "small" (5).
	v240 := &infodict.Format{VCodec: "vp09.00.20.08.01.05.01.06.00", ACodec: "none", Height: 240}
	v240.URL = "https://rr.example/playlist/itag/604/playlist.m3u8"
	if !(&Base{}).processManifestFormat(v240, "hls", "web", "604", st) {
		t.Fatal("240p format rejected")
	}
	if v240.Quality == nil || *v240.Quality != 5 {
		t.Errorf("604 quality = %v, want 5", v240.Quality)
	}
}

func TestProcessManifestFormatClientPenalties(t *testing.T) {
	st := newFormatState("")
	// web_safari pre-merged m3u8 audio formats are deprioritized.
	f := &infodict.Format{VCodec: "none"}
	f.URL = "https://rr.example/playlist/itag/233/playlist.m3u8"
	(&Base{}).processManifestFormat(f, "hls", "web_safari", "233", st)
	if f.SourcePreference == nil || *f.SourcePreference != -2 {
		t.Errorf("safari source_preference = %v, want -2", f.SourcePreference)
	}

	// DRM-protected HLS streams are marked 'maybe' and deprioritized.
	stLive := newFormatState("is_live")
	g := &infodict.Format{VCodec: "avc1.4D400B", ACodec: "none", Height: 144, HasDRM: true}
	g.URL = "https://rr.example/playlist/itag/269/playlist.m3u8"
	(&Base{}).processManifestFormat(g, "hls", "web", "269", stLive)
	if g.HasDRM != "maybe" {
		t.Errorf("has_drm = %q, want maybe", g.HasDRM)
	}
	if g.SourcePreference == nil || *g.SourcePreference != -6 {
		t.Errorf("drm source_preference = %v, want -6", g.SourcePreference)
	}
}

func TestProcessFormatStreamTinyVideoQuality(t *testing.T) {
	// Regression: a 144p video format with quality "tiny" and no
	// audioQuality must populate itag/res qualities (used later by HLS).
	b := &Base{}
	b.SetDownloader(newHarness())
	st := newFormatState("")
	format := map[string]any{
		"itag":         "160",
		"mimeType":     `video/mp4; codecs="avc1.4D400B,mp4a.40.2"`,
		"quality":      "tiny",
		"qualityLabel": "144p",
		"height":       144,
		"width":        192,
		"fps":          15,
	}
	f := b.processFormatStream(cloneMap(format), "web", 0, false, st)
	if f == nil {
		t.Fatal("processFormatStream returned nil")
	}
	if st.itagQualities["160"] != "tiny" {
		t.Errorf("itag_qualities[160] = %q, want tiny", st.itagQualities["160"])
	}
	if q, ok := st.nearestResQuality(144); !ok || q != "tiny" {
		t.Errorf("nearestResQuality(144) = %q,%v, want tiny,true", q, ok)
	}
	if f.Quality == nil || *f.Quality != 0 {
		t.Errorf("160 quality = %v, want 0", f.Quality)
	}
}

func keysOf(m map[string]*infodict.Format) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
