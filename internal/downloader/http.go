// Package downloader implements the download stage (HTTP and, later, HLS/DASH).
package downloader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	var offset int64
	cont := true
	if d.opts.ContinueDL != nil {
		cont = *d.opts.ContinueDL
	}
	if cont {
		if st, err := os.Stat(dest); err == nil {
			offset = st.Size()
		}
	}
	headers := map[string]string{}
	if offset > 0 {
		headers["Range"] = fmt.Sprintf("bytes=%d-", offset)
	}
	resp, err := d.client.Get(url, headers)
	if err != nil {
		d.emitError(info, dest, offset, err)
		return err
	}
	defer resp.Close()

	if offset > 0 && resp.StatusCode != 206 {
		offset = 0
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	out, err := os.OpenFile(dest, flags, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	d.sink.Progress(events.Progress{
		Info: info, Filename: dest, Status: events.StatusStarted,
		Downloaded: offset, Total: totalSize(resp, offset),
	})

	start := time.Now()
	downloaded := offset
	total := totalSize(resp, offset)
	last := time.Time{}
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body().Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				d.emitError(info, dest, downloaded, werr)
				return werr
			}
			downloaded += int64(n)
			now := time.Now()
			if now.Sub(last) >= progressInterval {
				last = now
				d.sink.Progress(progressEvent(info, dest, downloaded, total, start, events.StatusRunning))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			d.emitError(info, dest, downloaded, rerr)
			return rerr
		}
	}

	d.sink.Progress(progressEvent(info, dest, downloaded, total, start, events.StatusFinished))
	return nil
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
