package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// writeDescription writes the video description to a .description sidecar file
// next to the downloaded media (e.g. "Title [id].mp4.description"), mirroring
// yt-dlp --write-description.
func (y *YoutubeDL) writeDescription(info *infodict.InfoDict) error {
	path := strings.TrimSuffix(info.Filepath, filepath.Ext(info.Filepath)) + ".description"
	if err := EnsureDir(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(info.Description), 0o666); err != nil {
		return err
	}
	y.ToScreen("[info] Writing video description to: %s", filepath.Base(path))
	return nil
}
