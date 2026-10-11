package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// writeInfoJSON writes a .info.json sidecar file next to the downloaded media
// file (e.g. "Title [id].info.json"), mirroring yt-dlp --write-info-json.
func (y *YoutubeDL) writeInfoJSON(info *infodict.InfoDict, path string) error {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	dest := base + ".info.json"
	if err := EnsureDir(dest); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(dest, data, 0o666); err != nil {
		return err
	}
	y.ToScreen("[info] Writing video metadata to %s", filepath.Base(dest))
	return nil
}
