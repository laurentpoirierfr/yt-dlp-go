package downloader

import (
	"fmt"
	"io"
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
	progress []events.Progress
	last     events.Progress
}

func (r *recordingSink) Progress(p events.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, p.Status)
	r.progress = append(r.progress, p)
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

func TestHTTPDownloadRetriesAfterAbortedConnection(t *testing.T) {
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i)
	}
	var mu sync.Mutex
	served := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		served++
		mu.Unlock()
		rangeHdr := req.Header.Get("Range")
		if rangeHdr == "" {
			// First attempt: advertise the full size but only send the first
			// half, then abort -> the client hits an (unexpected) EOF.
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusOK)
			w.Write(payload[:len(payload)/2])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
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
	dest := filepath.Join(dir, "retry.bin")
	sink := &recordingSink{}
	d := NewHTTP(networking.NewClient(), options.DefaultOptions(), sink)
	info := &infodict.InfoDict{ID: "x", Filepath: dest}
	if err := d.Download(info, []infodict.Format{{URL: srv.URL, Ext: "bin"}}); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: %d bytes got, %d want", len(got), len(payload))
	}
	mu.Lock()
	defer mu.Unlock()
	if served < 2 {
		t.Fatalf("expected a retry, server served %d times", served)
	}
	seq := sink.sequence()
	if seq[len(seq)-1] != events.StatusFinished {
		t.Fatalf("last event should be Finished, got %v", seq)
	}
	if sink.last.Downloaded != int64(len(payload)) {
		t.Fatalf("downloaded = %d want %d", sink.last.Downloaded, len(payload))
	}
}

func TestIsRetryable(t *testing.T) {
	if !isRetryable(io.ErrUnexpectedEOF) {
		t.Error("unexpected EOF should be retryable")
	}
	if !isRetryable(fmt.Errorf("read tcp x.y: read: %w", io.ErrUnexpectedEOF)) {
		t.Error("wrapped unexpected EOF should be retryable")
	}
	if isRetryable(fmt.Errorf("unexpected HTTP status 403")) {
		t.Error("403 should not be retryable")
	}
	if isRetryable(nil) {
		t.Error("nil should not be retryable")
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

func TestHTTPDownloadFragments(t *testing.T) {
	parts := []string{"abc", "defg", "hi"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var idx int
		fmt.Sscanf(req.URL.Path, "/frag%d.bin", &idx)
		if idx < 0 || idx >= len(parts) {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(parts[idx])))
		io.WriteString(w, parts[idx])
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "frag.bin")
	sink := &recordingSink{}
	d := NewHTTP(networking.NewClient(), options.DefaultOptions(), sink)

	info := &infodict.InfoDict{ID: "frag", Filepath: dest}
	f := infodict.Format{
		Ext:             "bin",
		FragmentBaseURL: srv.URL + "/",
		Fragments: []infodict.Fragment{
			{URL: "frag0.bin"}, {URL: "frag1.bin"}, {URL: "frag2.bin"},
		},
	}
	if err := d.Download(info, []infodict.Format{f}); err != nil {
		t.Fatalf("download: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "abcdefghi" {
		t.Fatalf("content = %q want %q", got, "abcdefghi")
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	seen := map[int]bool{}
	for _, p := range sink.progress {
		if p.FragmentCount != 3 {
			t.Fatalf("FragmentCount = %d want 3 (%+v)", p.FragmentCount, p)
		}
		if p.FragmentIndex > 0 {
			seen[p.FragmentIndex] = true
		}
	}
	for i := 1; i <= 3; i++ {
		if !seen[i] {
			t.Fatalf("fragment index %d never reported (seen %v)", i, seen)
		}
	}
	if sink.last.Status != events.StatusFinished || sink.last.FragmentIndex != 3 {
		t.Fatalf("last event = %+v, want finished on fragment 3/3", sink.last)
	}
	if sink.last.Downloaded != int64(len("abcdefghi")) {
		t.Fatalf("downloaded = %d want %d", sink.last.Downloaded, len("abcdefghi"))
	}
}

func TestResolveFragmentURL(t *testing.T) {
	cases := []struct{ base, ref, want string }{
		{"https://cdn.example/hls/index.m3u8", "seg1.ts", "https://cdn.example/hls/seg1.ts"},
		{"https://cdn.example/hls/", "seg1.ts", "https://cdn.example/hls/seg1.ts"},
		{"https://cdn.example/hls/", "https://other.example/x.ts", "https://other.example/x.ts"},
		{"https://cdn.example/hls/", "", ""},
	}
	for _, c := range cases {
		if got := resolveFragmentURL(c.base, c.ref); got != c.want {
			t.Errorf("resolveFragmentURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}
