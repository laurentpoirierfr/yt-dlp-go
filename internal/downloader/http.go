// Package downloader implements the download stage (HTTP and, later, HLS/DASH).
package downloader

import (
	"errors"
	"fmt"
	"io"
	"net"
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
	if url == "" {
		return fmt.Errorf("no URL to download for format %q", f.FormatID)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
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
			d.sink.Progress(progressEvent(info, dest, downloaded, downloaded, start, events.StatusFinished))
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
				d.sink.Progress(progressEvent(info, dest, written, total, segStart, events.StatusRunning))
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

func progressEvent(info *infodict.InfoDict, dest string, downloaded, total int64, start time.Time, status events.Status) events.Progress {
	elapsed := time.Since(start)
	p := events.Progress{
		Info: info, Filename: dest, Status: status,
		Downloaded: downloaded, Total: total,
		Elapsed: elapsed, ETA: -1,
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
