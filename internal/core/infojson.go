package core

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// writeInfoJSON writes a .info.json sidecar file next to the downloaded media
// file (e.g. "Title [id].mp4.info.json"), mirroring yt-dlp --write-info-json.
func (y *YoutubeDL) writeInfoJSON(info *infodict.InfoDict, path string) error {
	if err := EnsureDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path+".info.json", data, 0o666); err != nil {
		return err
	}
	y.ToScreen("[info] Writing video metadata to %s", filepath.Base(path)+".info.json")
	return nil
}
