package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func thumbServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "exists.webp", "exists.jpg":
			_, _ = w.Write([]byte("img:" + filepath.Base(r.URL.Path)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestWriteThumbnailBest(t *testing.T) {
	srv := thumbServer()
	defer srv.Close()
	dir := t.TempDir()
	o := options.DefaultOptions()
	o.WriteThumbnail = options.ThumbnailBest
	o.Paths = map[string]string{"home": dir}
	ydl := New(o)
	ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error { return nil })
	info := mkVideo("vid1")
	info.Title = "T"
	info.Thumbnails = []infodict.Thumbnail{
		{URL: srv.URL + "/exists.jpg", Ext: "jpg"},
		{URL: srv.URL + "/missing.webp", Ext: "webp"},
	}
	if err := ydl.processInfo(info); err != nil {
		t.Fatalf("processInfo: %v", err)
	}
	// Highest preference (missing.webp) fails; falls back to exists.jpg.
	data, err := os.ReadFile(filepath.Join(dir, "T [vid1].jpg"))
	if err != nil {
		t.Fatalf("best thumbnail not written: %v", err)
	}
	if string(data) != "img:exists.jpg" {
		t.Errorf("content = %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(dir, "T [vid1].webp")); err == nil {
		t.Error("missing webp should not have been written")
	}
}

func TestWriteAllThumbnails(t *testing.T) {
	srv := thumbServer()
	defer srv.Close()
	dir := t.TempDir()
	o := options.DefaultOptions()
	o.WriteThumbnail = options.ThumbnailAll
	o.Paths = map[string]string{"home": dir}
	ydl := New(o)
	ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error { return nil })
	info := mkVideo("vid2")
	info.Title = "T2"
	info.Thumbnails = []infodict.Thumbnail{
		{URL: srv.URL + "/exists.jpg", Ext: "jpg"},
		{URL: srv.URL + "/missing.webp", Ext: "webp"},
		{URL: srv.URL + "/exists.webp", Ext: "webp"},
	}
	if err := ydl.processInfo(info); err != nil {
		t.Fatalf("processInfo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "T2 [vid2].0.jpg")); err != nil {
		t.Errorf("thumbnail 0 not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "T2 [vid2].1.webp")); err == nil {
		t.Error("missing thumbnail 1 should be skipped")
	}
	if _, err := os.Stat(filepath.Join(dir, "T2 [vid2].2.webp")); err != nil {
		t.Errorf("thumbnail 2 not written: %v", err)
	}
}

func TestThumbExtFromURL(t *testing.T) {
	got := thumbExt(infodict.Thumbnail{URL: "https://x/y.PNG"})
	if !strings.EqualFold(got, "png") {
		t.Errorf("thumbExt = %q, want png", got)
	}
	if got := thumbExt(infodict.Thumbnail{Ext: "webp", URL: "https://x/y.jpg"}); got != "webp" {
		t.Errorf("thumbExt = %q, want webp", got)
	}
	if got := thumbExt(infodict.Thumbnail{URL: "https://x/y"}); got != "jpg" {
		t.Errorf("thumbExt default = %q, want jpg", got)
	}
	if got := thumbExt(infodict.Thumbnail{URL: "https://x/y.jpg?sqp=abc&rs=1"}); got != "jpg" {
		t.Errorf("thumbExt with query = %q, want jpg", got)
	}
}
