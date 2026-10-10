package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

var fieldRe = regexp.MustCompile(`%\(([a-zA-Z_][a-zA-Z0-9_]*)\)([#0\- +]*[0-9]*(?:\.[0-9]+)?[sdifxXoOeE])?`)

// buildOutputPath renders the output template for an info dict.
func (y *YoutubeDL) buildOutputPath(info *infodict.InfoDict) (string, error) {
	tmpl := y.opts.Outtmpl["default"]
	if tmpl == "" {
		tmpl = "%(title)s [%(id)s].%(ext)s"
	}
	if y.opts.UseID {
		tmpl = "%(id)s.%(ext)s"
	}
	out := y.expandTemplate(tmpl, info)
	out = y.expandTemplate(out, info)
	out = filepath.FromSlash(out)
	if !filepath.IsAbs(out) {
		base := y.opts.Paths["home"]
		if base == "" {
			base = "."
		}
		out = filepath.Join(base, out)
	}
	return out, nil
}

func (y *YoutubeDL) expandTemplate(tmpl string, info *infodict.InfoDict) string {
	out := fieldRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := fieldRe.FindStringSubmatch(m)
		key := sub[1]
		val := infoField(info, key)
		return val
	})
	return out
}

// infoField returns the string value of a template field.
func infoField(info *infodict.InfoDict, key string) string {
	switch key {
	case "title":
		return info.Title
	case "id":
		if y := info.ID; y != "" {
			return y
		}
		return "NA"
	case "ext":
		if info.Ext != "" {
			return info.Ext
		}
		return "mp4"
	case "display_id":
		return info.DisplayID
	case "duration":
		if info.Duration > 0 {
			return formatSeconds(info.Duration)
		}
		return "NA"
	case "duration_string":
		return info.DurationString
	case "uploader":
		return info.Uploader
	case "uploader_id":
		return info.UploaderID
	case "channel":
		return info.Channel
	case "channel_id":
		return info.ChannelID
	case "upload_date":
		return info.UploadDate
	case "release_date":
		return info.ReleaseDate
	case "extractor":
		return info.Extractor
	case "extractor_key":
		return info.ExtractorKey
	case "webpage_url":
		return info.WebpageURL
	case "format_id":
		return info.FormatID
	case "format":
		return info.Format
	case "resolution":
		return info.Resolution
	case "height":
		if info.Height > 0 {
			return fmt.Sprintf("%d", info.Height)
		}
		return "NA"
	case "width":
		if info.Width > 0 {
			return fmt.Sprintf("%d", info.Width)
		}
		return "NA"
	case "playlist":
		return info.Playlist
	case "playlist_index":
		if info.PlaylistIndex != nil {
			return fmt.Sprintf("%d", *info.PlaylistIndex)
		}
		return "NA"
	case "playlist_id":
		return info.PlaylistID
	case "view_count":
		return utils.Str(info.ViewCount)
	case "like_count":
		return utils.Str(info.LikeCount)
	case "description":
		return info.Description
	}
	return ""
}

// sanitizeFilename removes path separators and illegal characters.
func sanitizeFilename(name string, restrict bool) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
			return '_'
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if restrict {
		name = restrictAscii(name)
	}
	if name == "" {
		name = "NA"
	}
	return name
}

func restrictAscii(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// EnsureDir creates the parent directory of path.
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
