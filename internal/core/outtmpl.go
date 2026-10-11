package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// fieldRe matches a %(field)s placeholder, optionally with a "|default"
// alternative and a printf-style conversion (e.g. "%(index)02d").
var fieldRe = regexp.MustCompile(`%\(([^)]*)\)([#0\- +]*[0-9]*(?:\.[0-9]+)?[sdifxXoOeEjJb]?)?`)

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
	out = y.sanitizePath(out)
	if !filepath.IsAbs(out) {
		base := y.opts.Paths["home"]
		if base == "" {
			base = "."
		}
		out = filepath.Join(base, out)
	}
	return out, nil
}

// OutputPath renders the output filename for an entry without downloading it,
// mirroring yt-dlp --get-filename.
func (y *YoutubeDL) OutputPath(info *infodict.InfoDict) (string, error) {
	return y.buildOutputPath(info)
}

// naPlaceholder is the value substituted for missing fields.
func (y *YoutubeDL) naPlaceholder() string {
	if y.opts.OuttmplNAPlaceholder != "" {
		return y.opts.OuttmplNAPlaceholder
	}
	return "NA"
}

func (y *YoutubeDL) expandTemplate(tmpl string, info *infodict.InfoDict) string {
	return fieldRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := fieldRe.FindStringSubmatch(m)
		spec, verb := sub[1], sub[2]
		if spec == "" {
			return ""
		}
		name, def, hasDef := spec, "", false
		if i := strings.IndexByte(spec, '|'); i >= 0 {
			name, def, hasDef = spec[:i], spec[i+1:], true
		}
		val, known := infoField(info, name)
		if val == "" && hasDef {
			val = y.expandTemplate(def, info)
		}
		if val == "" && !known {
			val = y.naPlaceholder()
		}
		if verb != "" {
			val = applyVerb(val, verb)
		}
		return y.sanitizeValue(val)
	})
}

// sanitizeValue cleans a substituted field value so it can never introduce a
// path separator or illegal character into the rendered name.
func (y *YoutubeDL) sanitizeValue(v string) string {
	if v == "" {
		return ""
	}
	return sanitizeFilename(v, y.opts.RestrictFilenames, y.opts.WindowsFilenames)
}

// applyVerb formats a value with a printf-style conversion suffix such as
// "02d", ".2f", "s" or "j".
func applyVerb(val, verb string) string {
	if verb == "" {
		return val
	}
	format := "%" + verb
	switch verb[len(verb)-1] {
	case 'j', 'J':
		b, err := json.Marshal(val)
		if err != nil {
			return val
		}
		return string(b)
	case 'd', 'i', 'o', 'O', 'x', 'X', 'b', 'c':
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return val
		}
		return fmt.Sprintf(format, n)
	case 'f', 'e', 'E', 'g', 'G':
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return val
		}
		return fmt.Sprintf(format, f)
	default: // 's'
		return fmt.Sprintf(format, val)
	}
}

// infoField returns the string value of a template field and whether the field
// is recognized and carries a value. A recognized-but-empty value (known true,
// value "") is left empty; a value-less numeric field (known false) falls back
// to the NA placeholder or the "|default" alternative.
func infoField(info *infodict.InfoDict, key string) (string, bool) {
	switch key {
	case "title":
		return info.Title, true
	case "id":
		return info.ID, info.ID != ""
	case "ext":
		if info.Ext != "" {
			return info.Ext, true
		}
		return "mp4", true
	case "display_id":
		return info.DisplayID, true
	case "duration":
		if info.Duration > 0 {
			return formatSeconds(info.Duration), true
		}
		return "", false
	case "duration_string":
		return info.DurationString, true
	case "uploader":
		return info.Uploader, true
	case "uploader_id":
		return info.UploaderID, true
	case "channel":
		return info.Channel, true
	case "channel_id":
		return info.ChannelID, true
	case "upload_date":
		return info.UploadDate, true
	case "release_date":
		return info.ReleaseDate, true
	case "extractor":
		return info.Extractor, true
	case "extractor_key":
		return info.ExtractorKey, true
	case "webpage_url":
		return info.WebpageURL, true
	case "format_id":
		return info.FormatID, true
	case "format":
		return info.Format, true
	case "resolution":
		return info.Resolution, true
	case "height":
		if info.Height > 0 {
			return fmt.Sprintf("%d", info.Height), true
		}
		return "", false
	case "width":
		if info.Width > 0 {
			return fmt.Sprintf("%d", info.Width), true
		}
		return "", false
	case "playlist":
		return info.Playlist, true
	case "playlist_title":
		return info.Playlist, true
	case "playlist_index":
		if info.PlaylistIndex != nil {
			return fmt.Sprintf("%d", *info.PlaylistIndex), true
		}
		return "", false
	case "playlist_id":
		return info.PlaylistID, true
	case "view_count":
		return utils.Str(info.ViewCount), true
	case "like_count":
		return utils.Str(info.LikeCount), true
	case "description":
		return info.Description, true
	}
	return "", false
}

// sanitizePath cleans every path component while preserving the directory
// structure produced by the template.
func (y *YoutubeDL) sanitizePath(p string) string {
	sep := string(os.PathSeparator)
	parts := strings.Split(p, sep)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = y.sanitizeComponent(part)
	}
	return strings.Join(parts, sep)
}

func (y *YoutubeDL) sanitizeComponent(name string) string {
	if y.opts.TrimFileName > 0 {
		name = trimRunes(name, y.opts.TrimFileName)
	}
	if name == "" {
		name = y.naPlaceholder()
	}
	return name
}

// sanitizeFilename replaces path separators and characters that are illegal on
// common filesystems. restrict keeps only ASCII [A-Za-z0-9._-]; windows also
// strips trailing dots and spaces.
func sanitizeFilename(name string, restrict, windows bool) string {
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
	if restrict {
		name = restrictAscii(name)
	}
	if windows {
		name = strings.TrimRight(name, ". ")
	}
	name = strings.TrimSpace(name)
	return name
}

func restrictAscii(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// EnsureDir creates the parent directory of path.
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
