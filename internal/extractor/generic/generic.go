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
	// reSchemeDomain mirrors Python r'[^\s/]+\.[^\s/]+/' used by the no-scheme
	// fixup path to detect domain-looking input.
	reSchemeDomain = regexp.MustCompile(`^[^\s/]+\.[^\s/]+/`)
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

// Suitable matches every URL (fallback extractor, mirroring yt-dlp's
// GenericIE._VALID_URL = r'.*').
func (ie *GenericIE) Suitable(string) bool { return true }

// RealExtract scrapes a page for a playable media URL.
func (ie *GenericIE) RealExtract(u string) (*infodict.InfoDict, error) {
	// Protocol-relative URL: re-dispatch with the preferred HTTP scheme.
	if strings.HasPrefix(u, "//") {
		return extractor.URLResult(ie.httpScheme()+u, "", "", ""), nil
	}
	// URLs without a scheme go through the default_search fixup. Only the
	// default mode ('fixup_error') is implemented.
	if urlScheme(u) == "" {
		if reSchemeDomain.MatchString(u) {
			ie.ReportWarning("The url doesn't specify the protocol, trying with https")
			return extractor.URLResult("https://"+u, "", "", ""), nil
		}
		return nil, ie.NewExtractorError(pyRepr(u)+" is not a valid URL", true, nil)
	}

	id := ie.GenericID(u)
	webpage, err := ie.DownloadWebpage(extractor.NewRequest(u, nil, nil), id, "Downloading webpage", "", true, "")
	if err != nil {
		return nil, err
	}
	if webpage == "" {
		return nil, ie.NewExtractorError("Unable to download webpage", true, nil)
	}

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

// httpScheme returns the scheme (with colon) to use for protocol-relative
// URLs, mirroring InfoExtractor.http_scheme.
func (ie *GenericIE) httpScheme() string {
	if ie.Opts().PreferInsecure {
		return "http:"
	}
	return "https:"
}

// urlScheme extracts the URL scheme like Python's urllib.parse.urlparse: the
// scheme is the leading run matching [A-Za-z][A-Za-z0-9+.-]* terminated by a
// colon that appears before any '/', '?' or '#'. Returns "" if none.
func urlScheme(u string) string {
	if u == "" || !isASCIIAlpha(u[0]) {
		return ""
	}
	for i := 1; i < len(u); i++ {
		c := u[i]
		if c == ':' {
			return strings.ToLower(u[:i])
		}
		if c == '/' || c == '?' || c == '#' {
			return ""
		}
		if !isSchemeChar(c) {
			return ""
		}
	}
	return ""
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSchemeChar(c byte) bool {
	return isASCIIAlpha(c) || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}

// pyRepr renders s like Python's repr() for the common single-line case, so
// error messages such as "{url!r} is not a valid URL" match yt-dlp.
func pyRepr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c == quote {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte(quote)
	return b.String()
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
