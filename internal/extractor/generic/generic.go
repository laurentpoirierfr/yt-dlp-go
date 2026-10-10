// Package generic implements the GenericIE fallback extractor.
package generic

import (
	"regexp"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

var (
	reVideoSrc  = regexp.MustCompile(`(?is)<video\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	reSourceSrc = regexp.MustCompile(`(?is)<source\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	reM3U8      = regexp.MustCompile(`(?i)https?://[^\s"'<>]+\.m3u8[^\s"'<>]*`)
	reMPD       = regexp.MustCompile(`(?i)https?://[^\s"'<>]+\.mpd[^\s"'<>]*`)
	reJSONURL   = regexp.MustCompile(`"(?:contentUrl|embedUrl|videoUrl)"\s*:\s*"([^"]+)"`)
)

// GenericIE is the last-resort extractor for arbitrary pages.
type GenericIE struct {
	extractor.Base
}

// NewGenericIE builds the generic extractor.
func NewGenericIE() *GenericIE {
	ie := &GenericIE{}
	ie.Name = "Generic"
	ie.Description = "Generic fallback extractor"
	ie.SetSelf(ie)
	return ie
}

func init() { extractor.Register(NewGenericIE()) }

// Suitable matches every http(s) URL (fallback extractor).
func (ie *GenericIE) Suitable(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// RealExtract scrapes a page for a playable media URL.
func (ie *GenericIE) RealExtract(u string) (*infodict.InfoDict, error) {
	webpage, err := ie.DownloadWebpage(extractor.NewRequest(u, nil, nil), "", "Downloading webpage", "", true, "")
	if err != nil {
		return nil, err
	}
	if webpage == "" {
		return nil, ie.NewExtractorError("Unable to download webpage", true, nil)
	}

	id := ie.GenericID(u)
	title := ie.GenericTitle(u, webpage)

	var formats []infodict.Format
	add := func(rawURL, proto string) {
		if rawURL == "" {
			return
		}
		ext := extOf(rawURL)
		f := infodict.Format{
			URL:      rawURL,
			Ext:      ext,
			FormatID: proto,
			Format:   proto,
			Protocol: proto,
		}
		if proto == "m3u8" || proto == "m3u8_native" {
			f.Ext = "mp4"
			f.Protocol = "m3u8_native"
		}
		if proto == "http_dash_segments" {
			f.Ext = "mp4"
		}
		formats = append(formats, f)
	}

	for _, m := range reVideoSrc.FindAllStringSubmatch(webpage, -1) {
		add(absURL(u, m[1]), "http")
	}
	for _, m := range reSourceSrc.FindAllStringSubmatch(webpage, -1) {
		add(absURL(u, m[1]), "http")
	}
	if og, _ := ie.OGSearchProperty("video", webpage, false); og != "" {
		add(og, "http")
	}
	if og, _ := ie.OGSearchProperty("video:url", webpage, false); og != "" {
		add(og, "http")
	}
	if og, _ := ie.OGSearchProperty("video:secure_url", webpage, false); og != "" {
		add(og, "http")
	}
	for _, m := range reJSONURL.FindAllStringSubmatch(webpage, -1) {
		add(absURL(u, m[1]), "http")
	}
	if m := reM3U8.FindString(webpage); m != "" {
		add(m, "m3u8")
	}
	if m := reMPD.FindString(webpage); m != "" {
		add(m, "http_dash_segments")
	}

	if len(formats) == 0 {
		return nil, ie.NewExtractorError("No media found on page", true, nil)
	}

	info := &infodict.InfoDict{
		ID:         id,
		Title:      title,
		WebpageURL: u,
		Extractor:  "Generic",
		Type:       "video",
		Formats:    formats,
	}
	if d := ie.OGSearchDescription(webpage); d != "" {
		info.Description = d
	}
	if t := ie.OGSearchThumbnail(webpage); t != "" {
		info.Thumbnail = t
	}
	return info, nil
}

func absURL(base, ref string) string {
	if strings.HasPrefix(ref, "//") {
		if strings.HasPrefix(base, "https") {
			return "https:" + ref
		}
		return "http:" + ref
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	return ref
}

func extOf(u string) string {
	path := u
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndexByte(path, '.'); i >= 0 && i+1 < len(path) {
		return strings.ToLower(path[i+1:])
	}
	return "mp4"
}
