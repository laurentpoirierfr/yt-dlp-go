package youtube

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elfeo/yt-dlp-go/internal/extractor"
	"github.com/elfeo/yt-dlp-go/internal/infodict"
	"github.com/elfeo/yt-dlp-go/internal/networking"
	"github.com/elfeo/yt-dlp-go/internal/options"
)

// harness is a minimal extractor.Downloader for tests.
type harness struct {
	client *networking.Client
	opts   *options.Options
}

func newHarness() *harness {
	c := networking.NewClient()
	_ = c.ApplyOptions("", 30*time.Second, false, map[string]string{"Accept-Language": "en"}, "")
	return &harness{client: c, opts: options.DefaultOptions()}
}

func (h *harness) Client() *networking.Client       { return h.client }
func (h *harness) Options() *options.Options        { return h.opts }
func (h *harness) AllExtractors() []extractor.IE    { return extractor.All() }
func (h *harness) ToScreen(f string, a ...any)      { fmt.Fprintf(os.Stderr, "[info] "+f+"\n", a...) }
func (h *harness) ReportWarning(f string, a ...any) { fmt.Fprintf(os.Stderr, "WARNING: "+f+"\n", a...) }
func (h *harness) ReportError(f string, a ...any)   { fmt.Fprintf(os.Stderr, "ERROR: "+f+"\n", a...) }
func (h *harness) WriteDebug(f string, a ...any) {
	if os.Getenv("YTDL_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "debug: "+f+"\n", a...)
	}
}

func newTestIE() *YoutubeIE {
	ie := &YoutubeIE{}
	ie.Name = "Youtube"
	ie.ValidURLRe = validURLPatterns()
	ie.SetSelf(ie)
	return ie
}

func TestSuitable(t *testing.T) {
	ie := newTestIE()
	match := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42s",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ",
		"dQw4w9WgXcQ",
	}
	for _, u := range match {
		if !ie.Suitable(u) {
			t.Errorf("should match: %s", u)
		}
	}
	noMatch := []string{
		"https://www.youtube.com/@somechannel",
		"https://www.youtube.com/playlist?list=PL123",
		"https://example.com/watch?v=dQw4w9WgXcQ",
	}
	for _, u := range noMatch {
		if ie.Suitable(u) {
			t.Errorf("should not match: %s", u)
		}
	}
	if id := ie.MatchID("https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=1"); id != "dQw4w9WgXcQ" {
		t.Errorf("MatchID = %q", id)
	}
}

func TestRealExtract(t *testing.T) {
	if os.Getenv("YTDL_NET") == "" {
		t.Skip("set YTDL_NET=1 to run network tests")
	}
	ie := newTestIE()
	ie.SetDownloader(newHarness())
	info, err := ie.Extract("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	checkVideoInfo(t, info, "dQw4w9WgXcQ")
}

func checkVideoInfo(t *testing.T, info *infodict.InfoDict, wantID string) {
	t.Helper()
	if info.ID != wantID {
		t.Errorf("id = %q, want %q", info.ID, wantID)
	}
	if info.Title == "" {
		t.Error("empty title")
	}
	if len(info.Formats) == 0 {
		t.Fatal("no formats")
	}
	t.Logf("title: %q", info.Title)
	t.Logf("duration: %v, uploader: %q, views: %v", info.Duration, info.Uploader, info.ViewCount)
	t.Logf("live_status: %q, thumbs: %d", info.LiveStatus, len(info.Thumbnails))
	t.Logf("%d formats:", len(info.Formats))
	for _, f := range info.Formats {
		u := f.URL
		if len(u) > 70 {
			u = u[:70]
		}
		t.Logf("  id=%-6s ext=%-5s note=%-24s v=%-14s a=%-10s tbr=%.0f %s",
			f.FormatID, f.Ext, f.FormatNote, f.VCodec, f.ACodec, f.TBR, u)
	}
	for lang, tracks := range info.Subtitles {
		t.Logf("sub %s: %d tracks", lang, len(tracks))
	}
}
