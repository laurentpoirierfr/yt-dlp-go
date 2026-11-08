// Package downloader implements the download stage (HTTP and, later, HLS/DASH).
package downloader

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

// progressInterval is the minimum delay between StatusRunning events.
const progressInterval = 100 * time.Millisecond

// HTTPDownloader downloads plain HTTP(S) formats, with resume support.
type HTTPDownloader struct {
	client *networking.Client
	opts   *options.Options
	sink   events.Sink
}

// NewHTTP builds an HTTP downloader. sink may be nil (events are discarded).
func NewHTTP(client *networking.Client, opts *options.Options, sink events.Sink) *HTTPDownloader {
	if sink == nil {
		sink = events.NopSink{}
	}
	return &HTTPDownloader{client: client, opts: opts, sink: sink}
}

// Download writes the selected formats of info to disk. When several formats
// are requested (video+audio), they are written to temporary part files and
// the caller is expected to merge them; the part list is stored in
// info.RequestedDownloads.
func (d *HTTPDownloader) Download(info *infodict.InfoDict, formats []infodict.Format) error {
	if len(formats) == 0 {
		f := infodict.Format{URL: info.URL, Ext: info.Ext}
		return d.downloadOne(info, f, info.Filepath)
	}
	if len(formats) == 1 {
		return d.downloadOne(info, formats[0], info.Filepath)
	}
	for i, f := range formats {
		part := fmt.Sprintf("%s.f%d.%s", info.Filepath, i, orExt(f.Ext))
		if err := d.downloadOne(info, f, part); err != nil {
			return err
		}
		info.RequestedDownloads = append(info.RequestedDownloads, infodict.Format{
			URL: f.URL, Ext: f.Ext, FormatID: f.FormatID,
		})
	}
	return nil
}

func orExt(e string) string {
	if e == "" {
		return "dat"
	}
	return e
}

func (d *HTTPDownloader) downloadOne(info *infodict.InfoDict, f infodict.Format, dest string) error {
	url := f.URL
	if url == "" && len(f.Fragments) == 0 {
		return fmt.Errorf("no URL to download for format %q", f.FormatID)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	// Fragmented formats (HLS/DASH) are downloaded segment by segment; each
	// segment is appended to the destination file.
	if len(f.Fragments) > 0 {
		return d.downloadFragmented(info, f, dest)
	}

	cont := true
	if d.opts.ContinueDL != nil {
		cont = *d.opts.ContinueDL
	}
	if d.opts.Overwrites != nil && *d.opts.Overwrites {
		cont = false
	}
	offset := int64(0)
	if cont {
		if st, err := os.Stat(dest); err == nil {
			offset = st.Size()
		}
	}

	// Writes always append at the end of the file; a non-continue run truncates
	// any stale part first.
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if !cont {
		if err := file.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}

	maxRetries := d.opts.Retries
	if maxRetries < 0 {
		maxRetries = 0
	}
	downloaded := offset
	start := time.Now()
	for attempt := 0; ; attempt++ {
		written, rerr := d.downloadSegment(info, url, file, dest, downloaded)
		downloaded = written
		if rerr == nil {
			d.sink.Progress(progressEvent(info, dest, downloaded, downloaded, start, events.StatusFinished, 0, 0))
			return nil
		}
		if attempt >= maxRetries || !isRetryable(rerr) {
			d.emitError(info, dest, downloaded, rerr)
			return rerr
		}
		d.sink.Log(events.Log{
			Level: events.LevelWarning,
			Text:  fmt.Sprintf("[download] Got error: %v (retry %d of %d)", rerr, attempt+1, maxRetries),
		})
	}
}

// downloadFragmented downloads every fragment of f sequentially, appending it
// to dest. Failures are retried from scratch, truncating the destination.
func (d *HTTPDownloader) downloadFragmented(info *infodict.InfoDict, f infodict.Format, dest string) error {
	maxRetries := d.opts.Retries
	if maxRetries < 0 {
		maxRetries = 0
	}
	count := len(f.Fragments)
	total := fragmentsTotal(f.Fragments)
	start := time.Now()
	var downloaded int64
	for attempt := 0; ; attempt++ {
		written, rerr := d.writeFragments(info, f, dest, total, start)
		downloaded = written
		if rerr == nil {
			d.sink.Progress(progressEvent(info, dest, downloaded, downloaded, start, events.StatusFinished, count, count))
			return nil
		}
		if attempt >= maxRetries || !isRetryable(rerr) {
			d.emitError(info, dest, downloaded, rerr)
			return rerr
		}
		d.sink.Log(events.Log{
			Level: events.LevelWarning,
			Text:  fmt.Sprintf("[download] Got error: %v (retry %d of %d)", rerr, attempt+1, maxRetries),
		})
	}
}

// writeFragments truncates dest and writes all fragments to it, emitting
// fragment-aware progress events as it goes.
func (d *HTTPDownloader) writeFragments(info *infodict.InfoDict, f infodict.Format, dest string, total int64, start time.Time) (int64, error) {
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := len(f.Fragments)
	base := f.FragmentBaseURL
	if base == "" {
		base = f.URL
	}
	d.sink.Progress(events.Progress{
		Info: info, Filename: dest, Status: events.StatusStarted,
		Downloaded: 0, Total: total,
		FragmentIndex: 1, FragmentCount: count,
	})

	var written int64
	for i, frag := range f.Fragments {
		u := resolveFragmentURL(base, frag.URL)
		if u == "" {
			return written, fmt.Errorf("fragment %d/%d has no URL", i+1, count)
		}
		n, ferr := d.writeFragment(info, u, file, dest, i+1, count, total, written, start)
		written += n
		if ferr != nil {
			return written, ferr
		}
		d.sink.Progress(progressEvent(info, dest, written, total, start, events.StatusRunning, i+1, count))
	}
	return written, nil
}

// writeFragment fetches one fragment and appends its body to file.
func (d *HTTPDownloader) writeFragment(info *infodict.InfoDict, fragmentURL string, file *os.File, dest string, index, count int, total, written int64, start time.Time) (int64, error) {
	resp, err := d.client.Get(fragmentURL, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Close()

	switch resp.StatusCode {
	case 200, 206:
	default:
		return 0, fmt.Errorf("unexpected HTTP status %d while downloading fragment %d/%d", resp.StatusCode, index, count)
	}

	var n int64
	buf := make([]byte, 256*1024)
	last := time.Time{}
	for {
		r, rerr := resp.Body().Read(buf)
		if r > 0 {
			if _, werr := file.Write(buf[:r]); werr != nil {
				return n, werr
			}
			n += int64(r)
			now := time.Now()
			if last.IsZero() || now.Sub(last) >= progressInterval {
				last = now
				d.sink.Progress(progressEvent(info, dest, written+n, total, start, events.StatusRunning, index, count))
			}
		}
		if rerr == io.EOF {
			return n, nil
		}
		if rerr != nil {
			return n, rerr
		}
	}
}

// fragmentsTotal sums known fragment sizes, returning -1 when any is unknown.
func fragmentsTotal(frags []infodict.Fragment) int64 {
	var total int64
	for _, fr := range frags {
		if fr.Filesize <= 0 {
			return -1
		}
		total += fr.Filesize
	}
	if total == 0 {
		return -1
	}
	return total
}

// resolveFragmentURL resolves a fragment reference against its base URL.
func resolveFragmentURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if r.IsAbs() {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// downloadSegment fetches url and appends the body - or the remaining part of
// it, when start > 0 - to file. It returns the absolute number of bytes
// written (including start) and any read error.
func (d *HTTPDownloader) downloadSegment(info *infodict.InfoDict, url string, file *os.File, dest string, start int64) (int64, error) {
	headers := map[string]string{}
	if start > 0 {
		headers["Range"] = fmt.Sprintf("bytes=%d-", start)
	}
	resp, err := d.client.Get(url, headers)
	if err != nil {
		return start, err
	}
	defer resp.Close()

	if start > 0 {
		switch resp.StatusCode {
		case 206: // partial content, exactly what we asked
		case 200:
			// Server ignored the Range request; restart the part from scratch.
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return start, err
			}
			if err := file.Truncate(0); err != nil {
				return start, err
			}
			start = 0
		case 416: // range not satisfiable: the part is already complete
			return start, nil
		default:
			return start, fmt.Errorf("unexpected HTTP status %d while downloading format %s", resp.StatusCode, info.FormatID)
		}
	}

	total := totalSize(resp, start)
	if start == 0 {
		d.sink.Progress(events.Progress{
			Info: info, Filename: dest, Status: events.StatusStarted,
			Downloaded: 0, Total: total,
		})
	} else {
		d.sink.Progress(events.Progress{
			Info: info, Filename: dest, Status: events.StatusRunning,
			Downloaded: start, Total: total,
		})
	}

	written := start
	buf := make([]byte, 256*1024)
	segStart := time.Now()
	last := time.Time{}
	for {
		n, rerr := resp.Body().Read(buf)
		if n > 0 {
			if _, werr := file.Write(buf[:n]); werr != nil {
				return written, werr
			}
			written += int64(n)
			now := time.Now()
			if last.IsZero() || now.Sub(last) >= progressInterval {
				last = now
				d.sink.Progress(progressEvent(info, dest, written, total, segStart, events.StatusRunning, 0, 0))
			}
		}
		if rerr == io.EOF {
			return written, nil
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

// isRetryable reports whether a download error is worth retrying (transient
// network failures), as opposed to fatal protocol errors.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func totalSize(resp *networking.Response, offset int64) int64 {
	if cl := resp.HeaderGet("Content-Length"); cl != "" {
		var n int64
		fmt.Sscan(cl, &n)
		if n > 0 {
			return offset + n
		}
	}
	return -1
}

func progressEvent(info *infodict.InfoDict, dest string, downloaded, total int64, start time.Time, status events.Status, fragIndex, fragCount int) events.Progress {
	elapsed := time.Since(start)
	p := events.Progress{
		Info: info, Filename: dest, Status: status,
		Downloaded: downloaded, Total: total,
		Elapsed: elapsed, ETA: -1,
		FragmentIndex: fragIndex, FragmentCount: fragCount,
	}
	if elapsed > 0 {
		p.Speed = float64(downloaded) / elapsed.Seconds()
		if total > 0 && p.Speed > 0 {
			remaining := total - downloaded
			if remaining > 0 {
				p.ETA = time.Duration(float64(remaining)/p.Speed) * time.Second
			} else {
				p.ETA = 0
			}
		}
	}
	return p
}

func (d *HTTPDownloader) emitError(info *infodict.InfoDict, dest string, downloaded int64, err error) {
	d.sink.Progress(events.Progress{
		Info: info, Filename: dest, Status: events.StatusError,
		Downloaded: downloaded, Total: -1, Err: err,
	})
}
