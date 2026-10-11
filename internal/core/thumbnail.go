package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

// thumbnailsRequested reports whether a thumbnail-writing mode is set.
func (y *YoutubeDL) thumbnailsRequested() bool {
	return y.opts.WriteThumbnail != options.ThumbnailNone
}

// writeThumbnails writes the selected thumbnail image(s) next to the media
// file, mirroring yt-dlp --write-thumbnail / --write-all-thumbnails. Thumbnails
// are ordered from lowest to highest preference, so the last entry is the best.
func (y *YoutubeDL) writeThumbnails(info *infodict.InfoDict) {
	if len(info.Thumbnails) == 0 {
		if info.Thumbnail == "" {
			y.ReportWarning("There are no thumbnails to write")
			return
		}
		info.Thumbnails = []infodict.Thumbnail{{URL: info.Thumbnail}}
	}
	base := strings.TrimSuffix(info.Filepath, filepath.Ext(info.Filepath))
	if y.opts.WriteThumbnail == options.ThumbnailAll {
		for i, t := range info.Thumbnails {
			y.writeThumbnail(fmt.Sprintf("%s.%d.%s", base, i, thumbExt(t)), t)
		}
		return
	}
	// Best: try from the highest preference down until one downloads.
	for i := len(info.Thumbnails) - 1; i >= 0; i-- {
		t := info.Thumbnails[i]
		if y.writeThumbnail(base+"."+thumbExt(t), t) {
			return
		}
	}
}

// writeThumbnail downloads t to path and reports whether it succeeded.
func (y *YoutubeDL) writeThumbnail(path string, t infodict.Thumbnail) bool {
	data, _, err := y.client.DownloadTo(t.URL, nil)
	if err != nil {
		y.ReportWarning("Unable to download thumbnail %s: %v", t.URL, err)
		return false
	}
	if err := EnsureDir(path); err != nil {
		y.ReportWarning("Unable to write thumbnail: %v", err)
		return false
	}
	if err := os.WriteFile(path, data, 0o666); err != nil {
		y.ReportWarning("Unable to write thumbnail: %v", err)
		return false
	}
	y.ToScreen("[info] Writing video thumbnail to: %s", filepath.Base(path))
	return true
}

// thumbExt resolves the image extension of a thumbnail.
func thumbExt(t infodict.Thumbnail) string {
	if t.Ext != "" {
		return t.Ext
	}
	u := t.URL
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(u)), "."); ext != "" {
		return ext
	}
	return "jpg"
}
