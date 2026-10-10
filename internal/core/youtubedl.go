// Package core implements the extraction/download pipeline, the Go port of
// yt-dlp's YoutubeDL class. It ties together the extractor registry, the
// options, the networking client and the downloaders.
package core

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/render"
)

// YoutubeDL is the central pipeline object.
type YoutubeDL struct {
	opts   *options.Options
	client *networking.Client
	sink   events.Sink

	mu          sync.Mutex
	extractors  []extractor.IE
	byKey       map[string]extractor.IE
	playlistURL map[string]bool
	playlistLvl int

	archiveMu sync.Mutex
	archive   map[string]bool

	// statistics
	Errors   int
	Warnings int
	Success  int

	downloader DownloadFunc
}

// New builds a YoutubeDL with the given options.
func New(opts *options.Options) *YoutubeDL {
	if opts == nil {
		opts = options.DefaultOptions()
	}
	ydl := &YoutubeDL{
		opts:        opts,
		byKey:       map[string]extractor.IE{},
		playlistURL: map[string]bool{},
	}
	ydl.sink = render.NewPlainSink(opts)
	ydl.initClient()
	return ydl
}

// SetSink installs the event sink (plain renderer by default; a TUI swaps in
// its own). Must be called before extraction begins.
func (y *YoutubeDL) SetSink(s events.Sink) {
	if s == nil {
		s = events.NopSink{}
	}
	y.sink = s
}

// Sink returns the current event sink.
func (y *YoutubeDL) Sink() events.Sink { return y.sink }

func (y *YoutubeDL) initClient() {
	headers := map[string]string{}
	for k, v := range y.opts.HTTPHeaders {
		headers[k] = v
	}
	if y.opts.UserAgent != "" {
		headers["User-Agent"] = y.opts.UserAgent
	}
	c := networking.NewClient()
	_ = c.ApplyOptions(y.opts.Proxy, y.opts.SocketTimeout, y.opts.NoCheckCertificate, headers, y.opts.UserAgent)
	y.client = c
}

// RegisterExtractors installs the available extractors (called by the CLI with
// the blank-imported extractor packages' instances).
func (y *YoutubeDL) RegisterExtractors(ies []extractor.IE) {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.extractors = ies
	y.byKey = make(map[string]extractor.IE, len(ies))
	for _, ie := range ies {
		y.byKey[ie.IEKey()] = ie
	}
}

// ---------------------------------------------------------------------------
// extractor.Downloader implementation
// ---------------------------------------------------------------------------

// Client returns the shared HTTP client.
func (y *YoutubeDL) Client() *networking.Client { return y.client }

// Options returns the runtime options.
func (y *YoutubeDL) Options() *options.Options { return y.opts }

// AllExtractors returns the registered extractors.
func (y *YoutubeDL) AllExtractors() []extractor.IE {
	y.mu.Lock()
	defer y.mu.Unlock()
	out := make([]extractor.IE, len(y.extractors))
	copy(out, y.extractors)
	return out
}

// ToScreen prints an informational message.
func (y *YoutubeDL) ToScreen(format string, args ...any) {
	y.sink.Log(events.Log{Level: events.LevelInfo, Text: fmt.Sprintf(format, args...)})
}

// ReportWarning prints a warning.
func (y *YoutubeDL) ReportWarning(format string, args ...any) {
	y.mu.Lock()
	y.Warnings++
	y.mu.Unlock()
	y.sink.Log(events.Log{Level: events.LevelWarning, Text: fmt.Sprintf(format, args...)})
}

// ReportError prints an error and increments the error count.
func (y *YoutubeDL) ReportError(format string, args ...any) {
	y.mu.Lock()
	y.Errors++
	y.mu.Unlock()
	y.sink.Log(events.Log{Level: events.LevelError, Text: fmt.Sprintf(format, args...)})
}

// WriteDebug prints a debug message when verbose.
func (y *YoutubeDL) WriteDebug(format string, args ...any) {
	y.sink.Log(events.Log{Level: events.LevelDebug, Text: fmt.Sprintf(format, args...)})
}

// ---------------------------------------------------------------------------
// Extractor lookup
// ---------------------------------------------------------------------------

// getInfoExtractor returns the extractor matching url, or nil.
func (y *YoutubeDL) getInfoExtractor(url string, ieKey string) extractor.IE {
	if ieKey != "" {
		ie, ok := y.byKey[strings.ToLower(ieKey)]
		if !ok {
			return nil
		}
		if ie.Suitable(url) {
			return ie
		}
		return nil
	}
	for _, ie := range y.AllExtractors() {
		if ie.Suitable(url) {
			return ie
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Info post-processing helpers
// ---------------------------------------------------------------------------

// sanitizeInfo prepares a fresh copy of an info dict for processing.
func sanitizeInfo(info *infodict.InfoDict) *infodict.InfoDict {
	if info == nil {
		return nil
	}
	return info.Clone()
}

// sortThumbnails orders thumbnails by preference/width/height/id/url.
func (y *YoutubeDL) sortThumbnails(info *infodict.InfoDict) {
	if len(info.Thumbnails) == 0 {
		return
	}
	sort.SliceStable(info.Thumbnails, func(i, j int) bool {
		a, b := info.Thumbnails[i], info.Thumbnails[j]
		pa, pb := thaw(a.Preference), thaw(b.Preference)
		if pa != pb {
			return pa < pb
		}
		return a.URL < b.URL
	})
	for i := range info.Thumbnails {
		t := &info.Thumbnails[i]
		if t.ID == "" {
			t.ID = fmt.Sprintf("%d", i)
		}
		if t.Width > 0 && t.Height > 0 {
			t.Resolution = fmt.Sprintf("%dx%d", t.Width, t.Height)
		}
	}
	if info.Thumbnail == "" && len(info.Thumbnails) > 0 {
		info.Thumbnail = info.Thumbnails[len(info.Thumbnails)-1].URL
	}
}

func thaw(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
