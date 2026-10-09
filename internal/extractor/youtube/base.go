package youtube

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/elfeo/yt-dlp-go/internal/extractor"
	"github.com/elfeo/yt-dlp-go/internal/infodict"
)

// Base carries YouTube-specific machinery shared by all YouTube extractors.
type Base struct {
	extractor.Base

	// per-extraction caches
	ytcfgs      map[string]map[string]any
	playerURL   string
	playerJS    string
	playerID    string
	sigSpecs    map[int]string // len(solved probe) probe -> solved (index spec as string)
	nResults    map[string]string
	firstWebreq bool
}

// IEKey implements extractor.IE.
func (b *Base) IEKey() string { return strings.ToLower(b.IEName()) }

// SetDownloader resets per-extraction state.
func (b *Base) SetDownloader(d extractor.Downloader) {
	b.Base.SetDownloader(d)
	b.ytcfgs = map[string]map[string]any{}
	b.sigSpecs = map[int]string{}
	b.nResults = map[string]string{}
}

// getYtcfg returns (and memoizes) the ytcfg for a client.
func (b *Base) getYtcfg(c *Client, videoID string) map[string]any {
	if cfg, ok := b.ytcfgs[c.Name]; ok {
		return cfg
	}
	cfg := b.downloadYtcfg(c, videoID)
	if cfg == nil {
		cfg = map[string]any{}
	}
	b.ytcfgs[c.Name] = cfg
	return cfg
}

// requestedClients parses the youtube:player_client extractor arg.
func (b *Base) requestedClients(authenticated bool) ([]string, error) {
	def := splitList(defaultClients)
	arg := b.GetExtractorArg("player_client")
	if len(arg) == 0 {
		return def, nil
	}
	var names []string
	for _, a := range arg {
		for _, n := range splitList(a) {
			if n == "default" {
				names = append(names, def...)
			} else if n == "all" {
				names = append(names, allClientNames()...)
			} else if strings.HasPrefix(n, "-") {
				excl := strings.TrimPrefix(n, "-")
				var kept []string
				for _, x := range names {
					if x != excl {
						kept = append(kept, x)
					}
				}
				names = kept
			} else {
				names = append(names, n)
			}
		}
	}
	names = orderedNames(names)
	if len(names) == 0 {
		return nil, b.NewExtractorError("No player clients have been requested", false, nil)
	}
	return names, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func orderedNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func allClientNames() []string {
	type pair struct {
		name string
		prio int
	}
	var pairs []pair
	for name, c := range InnertubeClients {
		pairs = append(pairs, pair{name, c.Priority})
	}
	// stable sort desc by priority (insertion order for ties: map order is
	// random, so sort deterministically by priority then name)
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0 && (pairs[j].prio > pairs[j-1].prio ||
			(pairs[j].prio == pairs[j-1].prio && pairs[j].name < pairs[j-1].name)); j-- {
			pairs[j], pairs[j-1] = pairs[j-1], pairs[j]
		}
	}
	var out []string
	for _, p := range pairs {
		out = append(out, p.name)
	}
	return out
}

// splitInnertubeClient splits "web_embedded.visionos" style names.
func splitInnertubeClient(name string) (client, base, variant string) {
	if i := strings.IndexByte(name, '.'); i > 0 {
		base = name[:i]
		variant = name[i+1:]
		if _, ok := InnertubeClients[name]; ok {
			return name, base, variant
		}
		return base, base, variant
	}
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name, name[:i], name[i+1:]
	}
	return name, name, ""
}

// ---------------------------------------------------------------------------
// Player URL / JS
// ---------------------------------------------------------------------------

var (
	rePlayerID       = regexp.MustCompile(`/s/player/([a-fA-F0-9]{8,})/`)
	rePlayerURLYTCfg = regexp.MustCompile(`(?:PLAYER_JS_URL|jsUrl)["']?\s*[:=]\s*["']([^"']+)["']`)
	reSTS            = regexp.MustCompile(`(?:signatureTimestamp|sts)\s*:\s*([0-9]{5})`)
	reIframePlayer   = regexp.MustCompile(`player\?/([0-9a-fA-F]{8})\?/`)
)

// extractPlayerURL finds the player JS URL from ytcfgs/webpage.
func (b *Base) extractPlayerURL(ytcfgs ...map[string]any) string {
	for _, cfg := range ytcfgs {
		if cfg == nil {
			continue
		}
		if v, ok := cfg["PLAYER_JS_URL"].(string); ok && v != "" {
			return normalizePlayerURL(v)
		}
		if wpc, ok := cfg["WEB_PLAYER_CONTEXT_CONFIGS"].(map[string]any); ok {
			for _, v := range wpc {
				if m, ok := v.(map[string]any); ok {
					if js, ok := m["jsUrl"].(string); ok && js != "" {
						return normalizePlayerURL(js)
					}
				}
			}
		}
	}
	return ""
}

func normalizePlayerURL(u string) string {
	if strings.HasPrefix(u, "/") {
		u = "https://www.youtube.com" + u
	}
	return u
}

var playerVariants = map[string]string{
	"main":    "player_ias.vflset/en_US/base.js",
	"tcc":     "player_ias_tcc.vflset/en_US/base.js",
	"tce":     "player_ias_tce.vflset/en_US/base.js",
	"es5":     "player_es5.vflset/en_US/base.js",
	"es6":     "player_es6.vflset/en_US/base.js",
	"es6_tcc": "player_es6_tcc.vflset/en_US/base.js",
	"es6_tce": "player_es6_tce.vflset/en_US/base.js",
	"tv":      "tv-player-ias.vflset/tv-player-ias.js",
	"tv_es6":  "tv-player-es6.vflset/tv-player-es6.js",
	"phone":   "player-plasma-ias-phone-en_US.vflset/base.js",
	"house":   "house_brand_player.vflset/en_US/base.js",
}

// loadPlayer downloads the player JS (cached).
func (b *Base) loadPlayer(videoID string) error {
	if b.playerJS != "" {
		return nil
	}
	if b.playerURL == "" {
		return nil
	}
	if hasSkip(b, "js") {
		b.playerURL = ""
		return nil
	}
	// keep only the id, reconstruct canonical URL (variant "main" default)
	vid := ""
	if m := rePlayerID.FindStringSubmatch(b.playerURL); m != nil {
		vid = m[1]
	}
	if vid == "" {
		return fmt.Errorf("could not extract player id from %s", b.playerURL)
	}
	b.playerID = vid
	path := playerVariants["main"]
	if args := b.GetExtractorArg("player_js_variant"); len(args) > 0 && args[0] != "" {
		if p, ok := playerVariants[args[0]]; ok {
			path = p
		}
	} else {
		// keep the variant already present in the URL when known
		for _, p := range playerVariants {
			if strings.Contains(b.playerURL, p) {
				path = p
				break
			}
		}
	}
	b.playerURL = "https://www.youtube.com/s/player/" + vid + "/" + path
	js, err := b.DownloadWebpage(extractor.NewRequest(b.playerURL, nil, nil), videoID,
		"Downloading player JS", "Unable to download player JS", false, "")
	if err != nil || js == "" {
		b.playerJS = ""
		return fmt.Errorf("unable to download player JS")
	}
	b.playerJS = js
	return nil
}

// signatureTimestamp extracts sts from the player JS.
func (b *Base) signatureTimestamp() int {
	if args := b.GetExtractorArg("player_js_version"); len(args) > 0 && args[0] != "" {
		if m := regexp.MustCompile(`^(\d+)`).FindStringSubmatch(args[0]); m != nil {
			v := 0
			for _, c := range m[1] {
				v = v*10 + int(c-'0')
			}
			return v
		}
	}
	if m := reSTS.FindStringSubmatch(b.playerJS); m != nil {
		v := 0
		for _, c := range m[1] {
			v = v*10 + int(c-'0')
		}
		return v
	}
	return 0
}

// initialPlayerResponseRegex extracts ytInitialPlayerResponse JSON.
var (
	reInitialPR   = regexp.MustCompile(`ytInitialPlayerResponse\s*=\s*(\{.+?\})\s*;\s*(?:var\s|</script>|\n)`)
	reInitialData = regexp.MustCompile(`ytInitialData\s*=\s*(\{.+?\})\s*;\s*(?:var\s|</script>|\n)`)
)

func extractInitialJSON(re *regexp.Regexp, page string) map[string]any {
	m := re.FindStringSubmatch(page)
	if m == nil {
		return nil
	}
	v, err := extractor.ParseJSON(m[1], "", "initial data")
	if err != nil {
		return nil
	}
	mm, _ := v.(map[string]any)
	return mm
}

var _ = infodict.InfoDict{}
