package generic

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
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
func (h *harness) WriteDebug(string, ...any)        {}

func newTestIE() *GenericIE {
	ie := NewGenericIE()
	ie.SetDownloader(newHarness())
	return ie
}

func TestSuitableMatchesEverything(t *testing.T) {
	ie := NewGenericIE()
	for _, u := range []string{"https://example.com/", "not a url", "", "ftp://x/y", "//host/x"} {
		if !ie.Suitable(u) {
			t.Errorf("generic should match %q", u)
		}
	}
}

func TestURLScheme(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://x/y", "https"},
		{"http://x", "http"},
		{"//host/path", ""},
		{"not a url", ""},
		{"example.com/x", ""},
		{"1abc:foo", ""},
		{"ftp://example.com/video.mp4", "ftp"},
		{"javascript:alert(1)", "javascript"},
		{"file:///tmp/x.mp4", "file"},
		{"mailto:a@b.c", "mailto"},
		{"HTTP://X", "http"},
		{"", ""},
	}
	for _, c := range cases {
		if got := urlScheme(c.in); got != c.want {
			t.Errorf("urlScheme(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPyRepr(t *testing.T) {
	cases := []struct{ in, want string }{
		{"not a url", "'not a url'"},
		{"", "''"},
		{"it's", `"it's"`},
		{`a"b`, `'a"b'`},
		{`a\b`, `'a\\b'`},
	}
	for _, c := range cases {
		if got := pyRepr(c.in); got != c.want {
			t.Errorf("pyRepr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRealExtractNoScheme(t *testing.T) {
	ie := newTestIE()

	for _, in := range []string{"not a url", "foo bar", "", "1abc:foo", "example.com"} {
		_, err := ie.RealExtract(in)
		ee, ok := err.(*extractor.ExtractorError)
		if !ok {
			t.Fatalf("%q: expected *ExtractorError, got %T: %v", in, err, err)
		}
		want := pyRepr(in) + " is not a valid URL"
		if ee.Msg != want {
			t.Errorf("%q: msg = %q, want %q", in, ee.Msg, want)
		}
		if !ee.Expected {
			t.Errorf("%q: expected error must be expected=true", in)
		}
	}
}

func TestRealExtractFixupAndSchemeRelative(t *testing.T) {
	ie := newTestIE()
	cases := []struct{ in, wantURL string }{
		{"example.com/x", "https://example.com/x"},
		{"//media.w3.org/2010/05/sintel/trailer.mp4", "https://media.w3.org/2010/05/sintel/trailer.mp4"},
	}
	for _, c := range cases {
		res, err := ie.RealExtract(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if res == nil || res.Type != "url" || res.URL != c.wantURL {
			t.Errorf("%q: got %+v, want type=url url=%q", c.in, res, c.wantURL)
		}
	}
}
