package downloader

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

type recordingSink struct {
	mu       sync.Mutex
	statuses []events.Status
	last     events.Progress
}

func (r *recordingSink) Progress(p events.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, p.Status)
	r.last = p
}
func (r *recordingSink) PostProcess(events.PostProcess) {}
func (r *recordingSink) Log(events.Log)                 {}

func (r *recordingSink) sequence() []events.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]events.Status(nil), r.statuses...)
}

func TestHTTPDownloadEmitsProgress(t *testing.T) {
	payload := make([]byte, 1024*1024+7)
	for i := range payload {
		payload[i] = byte(i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "file.bin")
	sink := &recordingSink{}
	d := NewHTTP(networking.NewClient(), options.DefaultOptions(), sink)

	info := &infodict.InfoDict{ID: "x", Filepath: dest}
	if err := d.Download(info, []infodict.Format{{URL: srv.URL, Ext: "bin"}}); err != nil {
		t.Fatalf("download: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("size = %d want %d", len(got), len(payload))
	}

	seq := sink.sequence()
	if len(seq) < 2 || seq[0] != events.StatusStarted {
		t.Fatalf("first event should be Started, got %v", seq)
	}
	if seq[len(seq)-1] != events.StatusFinished {
		t.Fatalf("last event should be Finished, got %v", seq)
	}
	if sink.last.Downloaded != int64(len(payload)) {
		t.Fatalf("downloaded = %d want %d", sink.last.Downloaded, len(payload))
	}
	if sink.last.Total != int64(len(payload)) {
		t.Fatalf("total = %d want %d", sink.last.Total, len(payload))
	}
	if sink.last.Total != sink.last.Downloaded {
		t.Fatalf("finished event should be complete: %+v", sink.last)
	}
}

func TestHTTPDownloadResume(t *testing.T) {
	payload := []byte("0123456789abcdefghij")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rangeHdr := req.Header.Get("Range")
		if rangeHdr == "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.Write(payload)
			return
		}
		var start int
		fmt.Sscanf(rangeHdr, "bytes=%d-", &start)
		rest := payload[start:]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rest)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(rest)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "resume.bin")
	if err := os.WriteFile(dest, payload[:8], 0o644); err != nil {
		t.Fatal(err)
	}

	cont := true
	opts := options.DefaultOptions()
	opts.ContinueDL = &cont
	d := NewHTTP(networking.NewClient(), opts, events.NopSink{})
	info := &infodict.InfoDict{ID: "x", Filepath: dest}
	if err := d.Download(info, []infodict.Format{{URL: srv.URL, Ext: "bin"}}); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(payload) {
		t.Fatalf("got %q want %q", got, payload)
	}
}
