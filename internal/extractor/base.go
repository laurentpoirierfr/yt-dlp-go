package extractor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// Base implements the shared machinery of yt-dlp's InfoExtractor.
// Concrete extractors embed it and implement RealExtract.
type Base struct {
	downloader      Downloader
	ready           bool
	xForwardedForIP string
	videoID         string
	initializedOnce bool
	firstWebpageReq bool
	// self points at the concrete extractor instance so that the shared
	// Extract lifecycle can dispatch to its RealExtract (Go has no virtual
	// methods). It is set by NewInstance/SetSelf.
	self IE

	// ValidURLRe is the URL pattern; suitable() matches it against the start
	// of the URL (regex.match semantics). Nil means "never matches"
	// (embed-only extractors, _VALID_URL = False).
	ValidURLRe []*regexp.Regexp
	// Name is the IE name (class name - "IE"), e.g. "Youtube".
	Name string
	// NetrcMachine is the netrc machine name; empty means no login support.
	NetrcMachine string
	// Description is the IE description (--list-extractors).
	Description string

	// maxEntries caps how many playlist entries an extractor should collect
	// before stopping pagination (0 = fetch everything).
	maxEntries int
}

// SetPlaylistMaxEntries caps the number of playlist entries the extractor must
// collect (0 means no cap). It lets the core stop paginating as soon as the
// requested window (--playlist-start/end/items) is covered.
func (b *Base) SetPlaylistMaxEntries(n int) { b.maxEntries = n }

// PlaylistMaxEntries returns the current playlist entry cap.
func (b *Base) PlaylistMaxEntries() int { return b.maxEntries }

// IEKey returns the lowercase extractor key (e.g. "youtube").
func (b *Base) IEKey() string {
	if b.Name == "" {
		return "base"
	}
	return strings.ToLower(b.Name)
}

// IEName returns the extractor display name (e.g. "Youtube").
func (b *Base) IEName() string {
	if b.Name == "" {
		return "Base"
	}
	return b.Name
}

// SetDownloader attaches the core pipeline.
func (b *Base) SetDownloader(d Downloader) { b.downloader = d; b.firstWebpageReq = true }

// Downloader returns the attached pipeline.
func (b *Base) Downloader() Downloader { return b.downloader }

// Working reports whether the extractor is believed to work.
func (b *Base) Working() bool { return true }

// Opts returns the runtime options (nil-safe).
func (b *Base) Opts() *options.Options {
	if b.downloader == nil {
		return options.DefaultOptions()
	}
	return b.downloader.Options()
}

// GetExtractorArg returns --extractor-args IE:key values.
func (b *Base) GetExtractorArg(key string) []string {
	v, _ := b.Opts().GetExtractorArg(strings.ToLower(b.IEKey()), key)
	return v
}

// Suitable reports whether this extractor claims the URL.
func (b *Base) Suitable(u string) bool { return b.MatchValidURL(u) != nil }

// MatchValidURL returns the first matching regex for the URL.
func (b *Base) MatchValidURL(u string) *regexp.Regexp {
	for _, re := range b.ValidURLRe {
		if re.MatchString(u) {
			return re
		}
	}
	return nil
}

// MatchID returns the named "id" group of the URL match.
func (b *Base) MatchID(u string) string {
	re := b.MatchValidURL(u)
	if re == nil {
		return ""
	}
	m := re.FindStringSubmatch(u)
	if m == nil {
		return ""
	}
	for i, name := range re.SubexpNames() {
		if name == "id" && i < len(m) {
			return m[i]
		}
	}
	return ""
}

// Initialize runs the extractor lifecycle once (geo bypass + login + real init).
func (b *Base) Initialize() error {
	if b.ready {
		return nil
	}
	b.ready = true
	return nil
}

// SetSelf records the concrete extractor instance for method dispatch.
func (b *Base) SetSelf(ie IE) { b.self = ie }

// Extract is the public entry point: initialize, report, then RealExtract.
func (b *Base) Extract(u string) (*infodict.InfoDict, error) {
	if err := b.Initialize(); err != nil {
		return nil, err
	}
	b.ToScreen("Extracting URL: %s", u)
	b.videoID = b.MatchID(u)
	if b.self != nil {
		return b.self.RealExtract(u)
	}
	return b.RealExtract(u)
}

// RealExtract must be implemented by concrete extractors.
func (b *Base) RealExtract(string) (*infodict.InfoDict, error) {
	return nil, b.NewExtractorError("RealExtract not implemented", false, nil)
}

// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

// ToScreen prints an informational message.
func (b *Base) ToScreen(format string, args ...any) {
	if b.downloader != nil {
		b.downloader.ToScreen(format, args...)
	}
}

// ReportWarning prints a warning.
func (b *Base) ReportWarning(format string, args ...any) {
	if b.downloader != nil {
		b.downloader.ReportWarning(format, args...)
	}
}

// WriteDebug prints a debug message.
func (b *Base) WriteDebug(format string, args ...any) {
	if b.downloader != nil {
		b.downloader.WriteDebug(format, args...)
	}
}

// ReportDownloadWebpage prints the "Downloading webpage" note.
func (b *Base) ReportDownloadWebpage(videoID, note string) {
	if note == "" {
		note = "Downloading webpage"
	}
	b.ToScreen("[%s] %s", b.IEKey(), note)
}

// ---------------------------------------------------------------------------
// Networking pipeline
// ---------------------------------------------------------------------------

// Request describes an HTTP request made by an extractor.
type Request struct {
	URL     string
	Data    []byte
	Headers map[string]string
	Query   map[string]string
	Method  string
}

// NewRequest builds a request.
func NewRequest(u string, data []byte, headers map[string]string) *Request {
	return &Request{URL: u, Data: data, Headers: headers}
}

// WithQuery adds query parameters.
func (r *Request) WithQuery(q map[string]string) *Request {
	r.Query = q
	return r
}

func (b *Base) buildURL(u string, query map[string]string) string {
	if len(query) == 0 {
		return u
	}
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	q := pu.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	pu.RawQuery = q.Encode()
	return pu.String()
}

// RequestWebpage performs the request and returns the response handle.
func (b *Base) RequestWebpage(r *Request, videoID, note, errnote string, fatal bool, expectedStatus ...int) (*networking.Response, error) {
	if b.downloader == nil {
		return nil, fmt.Errorf("extractor not attached to a downloader")
	}
	if !b.firstWebpageReq {
		if si := b.Opts().SleepIntervalRequests; si > 0 {
			b.ToScreen("Sleeping %g seconds ...", si)
			time.Sleep(time.Duration(si * float64(time.Second)))
		}
	} else {
		b.firstWebpageReq = false
	}
	if note != "false" {
		b.ReportDownloadWebpage(videoID, note)
	}
	headers := map[string]string{}
	for k, v := range r.Headers {
		headers[k] = v
	}
	if b.xForwardedForIP != "" {
		if _, ok := headers["X-Forwarded-For"]; !ok {
			headers["X-Forwarded-For"] = b.xForwardedForIP
		}
	}
	client := b.downloader.Client()
	resp, err := client.Send(&networking.Request{
		URL:     b.buildURL(r.URL, r.Query),
		Data:    r.Data,
		Headers: headers,
		Method:  r.Method,
	}, expectedStatus...)
	if err != nil {
		errmsg := errnote
		if errmsg == "" {
			errmsg = "Unable to download webpage"
		}
		full := fmt.Sprintf("%s: %s", errmsg, err)
		if fatal {
			return nil, b.NewExtractorError(full, false, err)
		}
		b.ReportWarning("%s", full)
		return nil, nil
	}
	return resp, nil
}

// DownloadWebpageHandle downloads and returns (content, response).
func (b *Base) DownloadWebpageHandle(r *Request, videoID, note, errnote string, fatal bool, encoding string, expectedStatus ...int) (string, *networking.Response, error) {
	// Strip hashes from the URL (#1038)
	if i := strings.IndexByte(r.URL, '#'); i >= 0 {
		r.URL = r.URL[:i]
	}
	resp, err := b.RequestWebpage(r, videoID, note, errnote, fatal, expectedStatus...)
	if err != nil {
		return "", nil, err
	}
	if resp == nil {
		return "", nil, nil
	}
	return b.webpageReadContent(resp, videoID, note, errnote, fatal, encoding)
}

func (b *Base) webpageReadContent(resp *networking.Response, videoID, note, errnote string, fatal bool, encoding string) (string, *networking.Response, error) {
	data, err := io.ReadAll(resp.Body())
	resp.Body().Close()
	if err != nil {
		msg := errnote
		if msg == "" {
			msg = "Unable to download webpage"
		}
		full := fmt.Sprintf("%s: %s", msg, err)
		if fatal {
			return "", nil, b.NewExtractorError(full, false, err)
		}
		b.ReportWarning("%s", full)
		return "", nil, nil
	}
	if encoding == "" {
		encoding = guessEncoding(resp.HeaderGet("Content-Type"), data)
	}
	return decodeBytes(data, encoding), resp, nil
}

func guessEncoding(contentType string, data []byte) string {
	if m := regexp.MustCompile(`(?i)charset=["']?([^'";\s]+)`).FindStringSubmatch(contentType); m != nil {
		return strings.ToLower(m[1])
	}
	if len(data) > 1024 {
		data = data[:1024]
	}
	if m := regexp.MustCompile(`(?i)<meta[^>]+charset=["']?([^'")\s/]+)`).FindStringSubmatch(string(data)); m != nil {
		return strings.ToLower(m[1])
	}
	if len(data) >= 2 && data[0] == 0xff && data[1] == 0xfe {
		return "utf-16"
	}
	return "utf-8"
}

func decodeBytes(data []byte, encoding string) string {
	switch strings.ToLower(encoding) {
	case "", "utf-8", "utf8", "us-ascii":
		return string(data)
	case "latin1", "iso-8859-1", "windows-1252", "cp1252":
		var b strings.Builder
		for _, c := range data {
			b.WriteRune(rune(c))
		}
		return b.String()
	default:
		return string(data)
	}
}

// DownloadWebpage downloads a page and returns its text.
func (b *Base) DownloadWebpage(r *Request, videoID, note, errnote string, fatal bool, encoding string, expectedStatus ...int) (string, error) {
	text, _, err := b.DownloadWebpageHandle(r, videoID, note, errnote, fatal, encoding, expectedStatus...)
	return text, err
}

// ParseJSON parses JSON leniently (numbers as json.Number).
func ParseJSON(s, videoID, name string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return v, nil
}

// DownloadJSON downloads and parses a JSON document.
func (b *Base) DownloadJSON(r *Request, videoID, note, errnote string, fatal bool, expectedStatus ...int) (any, error) {
	text, err := b.DownloadWebpage(r, videoID, note, errnote, fatal, "", expectedStatus...)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	v, err := ParseJSON(text, videoID, note)
	if err != nil {
		if fatal {
			return nil, b.NewExtractorError(fmt.Sprintf("%s - %v", noteOr(errnote, "Unable to extract JSON"), err), false, err)
		}
		b.ReportWarning("Unable to parse JSON: %v", err)
		return nil, nil
	}
	return v, nil
}

func noteOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ---------------------------------------------------------------------------
// Search helpers
// ---------------------------------------------------------------------------

// SearchRegex performs yt-dlp's _search_regex.
// group: "" = first non-empty group (or whole match if no groups), "0" = whole
// match, otherwise a group name or index.
func (b *Base) SearchRegex(pattern any, s, name string, def *string, fatal bool, group string) (string, error) {
	var re *regexp.Regexp
	var match []string
	for _, p := range toStringPatterns(pattern) {
		cand := regexp.MustCompile(p)
		match = cand.FindStringSubmatch(s)
		if match != nil {
			re = cand
			break
		}
	}
	if match != nil && re != nil {
		switch {
		case group == "":
			if len(match) <= 1 {
				return match[0], nil
			}
			for i := 1; i < len(match); i++ {
				if match[i] != "" {
					return match[i], nil
				}
			}
			return match[0], nil
		case group == "0":
			return match[0], nil
		default:
			if idx, err := regexp.Compile(`^\d+$`); err == nil && idx.MatchString(group) {
				i := atoiSimple(group)
				if i >= 0 && i < len(match) {
					return match[i], nil
				}
			}
			for i, name := range re.SubexpNames() {
				if name == group && i < len(match) {
					return match[i], nil
				}
			}
		}
	}
	if def != nil {
		return *def, nil
	}
	if fatal {
		return "", &RegexNotFoundError{Msg: "Unable to extract " + name}
	}
	b.ReportWarning("unable to extract %s", name)
	return "", nil
}

func toStringPatterns(pattern any) []string {
	switch p := pattern.(type) {
	case string:
		return []string{p}
	case []string:
		return p
	case []any:
		var out []string
		for _, x := range p {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func atoiSimple(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// HTMLSearchRegex is SearchRegex + CleanHTML on the result.
func (b *Base) HTMLSearchRegex(pattern any, s, name string, def *string, fatal bool, group string) (string, error) {
	res, err := b.SearchRegex(pattern, s, name, def, fatal, group)
	if err != nil {
		return "", err
	}
	return utils.CleanHTML(res), nil
}

// SearchJSON finds a JSON blob starting after startPattern and parses it.
func (b *Base) SearchJSON(startPattern, s, name string, contains string, def map[string]any, fatal bool) (map[string]any, error) {
	if contains == "" {
		contains = `[{].*?[}]`
	}
	pat := fmt.Sprintf(`(?:%s)\s*(%s)\s*`, startPattern, contains)
	res, err := b.SearchRegex(pat, s, name, nil, false, "1")
	if err != nil {
		return nil, err
	}
	if res == "" {
		if def != nil {
			return def, nil
		}
		if fatal {
			return nil, &RegexNotFoundError{Msg: "Unable to extract " + name}
		}
		b.ReportWarning("unable to extract %s", name)
		return nil, nil
	}
	v, perr := ParseJSON(res, "", name)
	if perr != nil {
		if fatal {
			return nil, b.NewExtractorError(fmt.Sprintf("Unable to extract %s - Failed to parse JSON", name), false, perr)
		}
		b.ReportWarning("Unable to extract %s: %v", name, perr)
		return nil, nil
	}
	m, _ := v.(map[string]any)
	if m == nil && def != nil {
		return def, nil
	}
	return m, nil
}

// HTMLSearchMeta searches a <meta> tag by name.
func (b *Base) HTMLSearchMeta(name, html string) string {
	patterns := []string{
		`(?i)<meta[^>]+(?:(?:name|property|itemprop)\s*=\s*["']` + regexp.QuoteMeta(name) + `["'])[^>]+content\s*=\s*["']([^"']+)["']`,
		`(?i)<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+(?:(?:name|property|itemprop)\s*=\s*["']` + regexp.QuoteMeta(name) + `["'])`,
	}
	res, _ := b.SearchRegex(patterns, html, name, nil, false, "1")
	return res
}

// OGSearchProperty searches an OpenGraph meta property.
func (b *Base) OGSearchProperty(prop, html string, fatal bool) (string, error) {
	esc := regexp.QuoteMeta(prop)
	patterns := []string{
		`(?is)<meta[^>]+(?:property|itemprop)=` + esc + `[^>]+content=["']([^"']+)["']`,
		`(?is)<meta[^>]+content=["']([^"']+)["'][^>]+(?:property|itemprop)=` + esc,
	}
	res, err := b.SearchRegex(patterns, html, "og:"+prop, nil, fatal, "1")
	if err != nil {
		return "", err
	}
	return utils.UnescapeHTML(res), nil
}

// OGSearchTitle returns the og:title of a page.
func (b *Base) OGSearchTitle(html string) string {
	s, _ := b.OGSearchProperty("title", html, false)
	return s
}

// OGSearchDescription returns the og:description.
func (b *Base) OGSearchDescription(html string) string {
	s, _ := b.OGSearchProperty("description", html, false)
	return s
}

// OGSearchThumbnail returns the og:image.
func (b *Base) OGSearchThumbnail(html string) string {
	s, _ := b.OGSearchProperty("image", html, false)
	return s
}

// HTMLExtractTitle returns the <title> of a page.
func (b *Base) HTMLExtractTitle(html string) string {
	res, _ := b.HTMLSearchRegex(`(?s)<title\b[^>]*>([^<]+)</title>`, html, "title", nil, false, "1")
	return res
}

// GenericID returns an ID derived from the URL.
func (b *Base) GenericID(u string) string {
	u = strings.SplitN(u, "?", 2)[0]
	u = strings.TrimSuffix(u, "/")
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.LastIndexByte(u, '.'); i >= 0 {
		u = u[:i]
	}
	return url.PathEscape(u)
}

// GenericTitle returns og:title, falling back to <title> or the URL basename.
func (b *Base) GenericTitle(u, webpage string) string {
	if t := b.OGSearchTitle(webpage); t != "" {
		return t
	}
	if t := b.HTMLExtractTitle(webpage); t != "" {
		return t
	}
	if id := b.GenericID(u); id != "" {
		return id
	}
	return "video"
}

// ---------------------------------------------------------------------------
// Result builders
// ---------------------------------------------------------------------------

// URLResult builds a redirect result (_type=url).
func URLResult(u, ieKey, videoID, title string) *infodict.InfoDict {
	d := &infodict.InfoDict{Type: "url", URL: u, IEKey: ieKey, ID: videoID, Title: title}
	if ieKey == "Generic" {
		d.URL = SmuggleURL(u, map[string]any{"to_generic": true})
	}
	return d
}

// URLTransparentResult builds a transparent redirect result.
func URLTransparentResult(u, ieKey, videoID, title string) *infodict.InfoDict {
	d := URLResult(u, ieKey, videoID, title)
	d.Type = "url_transparent"
	return d
}

// PlaylistResult builds a playlist result.
func PlaylistResult(entries []*infodict.InfoDict, playlistID, title, description string) *infodict.InfoDict {
	return &infodict.InfoDict{
		Type:        "playlist",
		ID:          playlistID,
		Title:       title,
		Description: description,
		Entries:     entries,
	}
}

// SmuggleURL embeds JSON data into a URL fragment (yt-dlp's smuggle_url).
func SmuggleURL(u string, data map[string]any) string {
	b, _ := json.Marshal(data)
	return u + "#__youtubedl_smuggle=" + url.QueryEscape(string(b))
}

// UnsmuggleURL extracts smuggled data from a URL fragment.
func UnsmuggleURL(u string) (string, map[string]any) {
	const marker = "#__youtubedl_smuggle="
	i := strings.Index(u, marker)
	if i < 0 {
		return u, nil
	}
	raw, err := url.QueryUnescape(u[i+len(marker):])
	if err != nil {
		return u[:i], nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return u[:i], nil
	}
	return u[:i], data
}

// SetGeoBypassIP installs a fake X-Forwarded-For IP for geo restriction.
func (b *Base) SetGeoBypassIP(ip string) { b.xForwardedForIP = ip }

// XForwardedForIP returns the current fake IP.
func (b *Base) XForwardedForIP() string { return b.xForwardedForIP }

// RaiseGeoRestricted signals a geo restriction with optional countries.
func (b *Base) RaiseGeoRestricted(msg string, countries []string) error {
	return &GeoRestrictedError{Msg: msg, Countries: countries, IE: b.IEKey()}
}

// RaiseNoFormats signals "no formats found".
func (b *Base) RaiseNoFormats(msg string, expected bool) error {
	return &NoFormatsError{Msg: msg, Expected: expected, VideoID: b.videoID}
}

// Sleep sleeps for the given duration (used by retry helpers).
func (b *Base) Sleep(d time.Duration) { time.Sleep(d) }

// LocationRedirect returns the Location header of a response.
func LocationRedirect(resp *networking.Response) string {
	return resp.HeaderGet("Location")
}

// HTTPStatus exposes the response status code.
func HTTPStatus(resp *networking.Response) int { return resp.StatusCode }

// IsStatus reports whether the response has the given status.
func IsStatus(resp *networking.Response, code int) bool {
	return resp != nil && resp.StatusCode == code
}

// HeaderGet is a convenience wrapper.
func HeaderGet(resp *networking.Response, k string) string {
	if resp == nil {
		return ""
	}
	return resp.HeaderGet(k)
}
