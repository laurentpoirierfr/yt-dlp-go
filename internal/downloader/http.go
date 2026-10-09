// Package downloader implements the download stage (HTTP and, later, HLS/DASH).
package downloader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/elfeo/yt-dlp-go/internal/infodict"
	"github.com/elfeo/yt-dlp-go/internal/networking"
	"github.com/elfeo/yt-dlp-go/internal/options"
)

// HTTPDownloader downloads plain HTTP(S) formats, with resume support.
type HTTPDownloader struct {
	client *networking.Client
	opts   *options.Options
}

// NewHTTP builds an HTTP downloader.
func NewHTTP(client *networking.Client, opts *options.Options) *HTTPDownloader {
	return &HTTPDownloader{client: client, opts: opts}
}

// Download writes the selected formats of info to disk. When several formats
// are requested (video+audio), they are written to temporary part files and
// the caller is expected to merge them; the returned part list is stored in
// info via RequestedDownloads.
func (d *HTTPDownloader) Download(info *infodict.InfoDict, formats []infodict.Format) error {
	if len(formats) == 0 {
		f := infodict.Format{URL: info.URL, Ext: info.Ext}
		return d.downloadOne(info, f, info.Filepath)
	}
	if len(formats) == 1 {
		return d.downloadOne(info, formats[0], info.Filepath)
	}
	// multiple streams -> download parts
	for i, f := range formats {
		part := fmt.Sprintf("%s.f%d.%s", info.Filepath, i, orExt(f.Ext))
		if err := d.downloadOne(info, f, part); err != nil {
			return err
		}
		info.RequestedDownloads = append(info.RequestedDownloads, infodict.Format{
			URL: f.URL, Ext: f.Ext, FormatID: f.FormatID,
		})
		_ = part
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
		return fmt.Errorf("no URL to download for format %s", f.FormatID)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if d.opts.NoProgress == false {
		fmt.Fprintf(os.Stderr, "[download] Destination: %s\n", dest)
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
		return err
	}
	defer resp.Close()

	if offset > 0 && resp.StatusCode != 206 {
		// server ignored the range; restart
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

	total := resp.HeaderGet("Content-Length")
	start := time.Now()
	n, err := io.Copy(out, resp.Body())
	if err != nil {
		return err
	}
	if d.opts.NoProgress == false {
		elapsed := time.Since(start).Seconds()
		if elapsed <= 0 {
			elapsed = 0.001
		}
		fmt.Fprintf(os.Stderr, "[download] 100%% of %s in %s at %.2fKiB/s\n",
			humanSize(offset+n), humanDuration(time.Since(start)),
			float64(n)/1024/elapsed)
	}
	_ = total
	return nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func humanDuration(d time.Duration) string {
	secs := int(d.Seconds())
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
