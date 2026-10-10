package core

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// archiveKey builds the yt-dlp archive identifier ("extractor id") for an info
// dict, e.g. "youtube dQw4w9WgXcQ".
func archiveKey(info *infodict.InfoDict) string {
	ext := strings.ToLower(info.ExtractorKey)
	if ext == "" {
		ext = strings.ToLower(info.Extractor)
	}
	if ext == "" {
		ext = "generic"
	}
	return ext + " " + info.ID
}

// loadArchive reads --download-archive's file into the in-memory set (once).
func (y *YoutubeDL) loadArchive() {
	y.archiveMu.Lock()
	defer y.archiveMu.Unlock()
	if y.archive != nil {
		return
	}
	y.archive = map[string]bool{}
	if y.opts == nil || y.opts.DownloadArchive == "" {
		return
	}
	f, err := os.Open(y.opts.DownloadArchive)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		y.archive[strings.TrimSpace(sc.Text())] = true
	}
}

// inArchive reports whether the entry is already recorded in the archive.
func (y *YoutubeDL) inArchive(info *infodict.InfoDict) bool {
	if info == nil || info.ID == "" {
		return false
	}
	y.loadArchive()
	y.archiveMu.Lock()
	defer y.archiveMu.Unlock()
	return y.archive[archiveKey(info)]
}

// addArchive appends the entry to the --download-archive file (once).
func (y *YoutubeDL) addArchive(info *infodict.InfoDict) error {
	if info == nil || info.ID == "" {
		return nil
	}
	y.loadArchive()
	key := archiveKey(info)
	y.archiveMu.Lock()
	defer y.archiveMu.Unlock()
	if y.archive[key] {
		return nil
	}
	if y.opts.DownloadArchive == "" {
		return fmt.Errorf("no download archive configured")
	}
	if dir := path.Dir(y.opts.DownloadArchive); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(y.opts.DownloadArchive, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, key); err != nil {
		return err
	}
	y.archive[key] = true
	return nil
}
