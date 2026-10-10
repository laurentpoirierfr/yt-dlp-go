// Package youtube implements the YouTube extractor family.
package youtube

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
)

// StreamingProtocol mirrors yt-dlp's StreamingProtocol enum values.
type StreamingProtocol string

const (
	ProtoHTTPS StreamingProtocol = "https"
	ProtoDASH  StreamingProtocol = "dash"
	ProtoHLS   StreamingProtocol = "hls"
)

// GvsPoTokenPolicy mirrors GvsPoTokenPolicy.
type GvsPoTokenPolicy struct {
	Required                   bool
	Recommended                bool
	NotRequiredForPremium      bool
	NotRequiredWithPlayerToken bool
}

// Client is one Innertube client definition.
type Client struct {
	Name            string
	Host            string
	ClientName      string // e.g. WEB
	ClientVersion   string // e.g. 2.20260708.00.00
	ClientID        int    // X-YouTube-Client-Name
	UserAgent       string // extra client fields (userAgent / osName ...)
	ExtraClient     map[string]any
	SupportsCookies bool
	RequireJSPlayer bool
	GvsPolicies     map[StreamingProtocol]GvsPoTokenPolicy
	Priority        int
}

// StreamingProtocol policies used by web-family clients.
func webGvsPolicies() map[StreamingProtocol]GvsPoTokenPolicy {
	p := GvsPoTokenPolicy{Required: true, Recommended: false, NotRequiredForPremium: true}
	return map[StreamingProtocol]GvsPoTokenPolicy{
		ProtoHTTPS: p, ProtoDASH: p,
		ProtoHLS: {Required: false, NotRequiredForPremium: true},
	}
}

// InnertubeClients mirrors yt-dlp's INNERTUBE_CLIENTS (only the clients used
// by the default flows are fully modelled; others share the same shape).
var InnertubeClients = map[string]*Client{
	"web": {
		Name: "web", ClientName: "WEB", ClientVersion: "2.20260708.00.00", ClientID: 1,
		SupportsCookies: true, RequireJSPlayer: true, GvsPolicies: webGvsPolicies(), Priority: 30,
	},
	"web_safari": {
		Name: "web_safari", ClientName: "WEB", ClientVersion: "2.20260708.00.00", ClientID: 1,
		UserAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)",
		ExtraClient:     map[string]any{"userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)"},
		SupportsCookies: true, RequireJSPlayer: true, GvsPolicies: webGvsPolicies(), Priority: 30,
	},
	"web_embedded": {
		Name: "web_embedded", ClientName: "WEB_EMBEDDED_PLAYER", ClientVersion: "2.20260708.00.00", ClientID: 56,
		UserAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)",
		ExtraClient:     map[string]any{"userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)"},
		SupportsCookies: true, RequireJSPlayer: true, Priority: 30,
	},
	"visionos": {
		Name: "visionos", ClientName: "VISIONOS", ClientVersion: "1.02", ClientID: 101,
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_7_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15",
		ExtraClient: map[string]any{
			"deviceMake": "Apple", "deviceModel": "RealityDevice17,1",
			"osName": "visionOS", "osVersion": "26.5.23O471",
		},
		RequireJSPlayer: false, Priority: -10,
	},
	"mweb": {
		Name: "mweb", ClientName: "MWEB", ClientVersion: "2.20260708.05.00", ClientID: 2,
		UserAgent:       "Mozilla/5.0 (iPad; CPU OS 16_7_10 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1,gzip(gfe)",
		ExtraClient:     map[string]any{"userAgent": "Mozilla/5.0 (iPad; CPU OS 16_7_10 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1,gzip(gfe)"},
		SupportsCookies: true, RequireJSPlayer: true, GvsPolicies: webGvsPolicies(), Priority: 20,
	},
	"tv": {
		Name: "tv", ClientName: "TVHTML5", ClientVersion: "7.20260707.07.00", ClientID: 7,
		UserAgent:       "Mozilla/5.0 (ChromiumStylePlatform) Cobalt/25.lts.30.1034943-gold (unlike Gecko), Unknown_TV_Unknown_0/Unknown (Unknown, Unknown)",
		ExtraClient:     map[string]any{"userAgent": "Mozilla/5.0 (ChromiumStylePlatform) Cobalt/25.lts.30.1034943-gold (unlike Gecko), Unknown_TV_Unknown_0/Unknown (Unknown, Unknown)"},
		SupportsCookies: true, RequireJSPlayer: true, Priority: 40,
	},
	"tv_downgraded": {
		Name: "tv_downgraded", ClientName: "TVHTML5", ClientVersion: "5.20260707", ClientID: 7,
		UserAgent:       "Mozilla/5.0 (ChromiumStylePlatform) Cobalt/Version",
		ExtraClient:     map[string]any{"userAgent": "Mozilla/5.0 (ChromiumStylePlatform) Cobalt/Version"},
		SupportsCookies: true, RequireJSPlayer: true, Priority: 40,
	},
	"tv_simply": {
		Name: "tv_simply", ClientName: "TVHTML5_SIMPLY", ClientVersion: "1.0", ClientID: 75,
		RequireJSPlayer: true, Priority: 40,
	},
	"android": {
		Name: "android", ClientName: "ANDROID", ClientVersion: "21.26.364", ClientID: 3,
		UserAgent:       "com.google.android.youtube/21.26.364 (Linux; U; Android 11) gzip",
		ExtraClient:     map[string]any{"androidSdkVersion": 30, "osName": "Android", "osVersion": "11"},
		RequireJSPlayer: false, Priority: 10,
		GvsPolicies: map[StreamingProtocol]GvsPoTokenPolicy{
			ProtoHTTPS: {Required: true, NotRequiredWithPlayerToken: true},
			ProtoDASH:  {Required: true, NotRequiredWithPlayerToken: true},
			ProtoHLS:   {Required: false, NotRequiredWithPlayerToken: true},
		},
	},
	"android_vr": {
		Name: "android_vr", ClientName: "ANDROID_VR", ClientVersion: "1.65.10", ClientID: 28,
		UserAgent: "com.google.android.apps.youtube.vr.oculus/1.65.10 (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip",
		ExtraClient: map[string]any{
			"deviceMake": "Oculus", "deviceModel": "Quest 3", "androidSdkVersion": 32,
			"osName": "Android", "osVersion": "12L",
		},
		RequireJSPlayer: false, Priority: 10,
	},
	"ios": {
		Name: "ios", ClientName: "IOS", ClientVersion: "21.26.4", ClientID: 5,
		UserAgent: "com.google.ios.youtube/21.26.4 (iPhone16,2; U; CPU iOS 18_3_2 like Mac OS X;)",
		ExtraClient: map[string]any{
			"deviceMake": "Apple", "deviceModel": "iPhone16,2",
			"osName": "iPhone", "osVersion": "18.3.2.22D82",
		},
		RequireJSPlayer: false, Priority: 0,
		GvsPolicies: map[StreamingProtocol]GvsPoTokenPolicy{
			ProtoHTTPS: {Required: true, NotRequiredWithPlayerToken: true},
			ProtoDASH:  {Required: true, NotRequiredWithPlayerToken: true},
			ProtoHLS:   {Required: false, NotRequiredWithPlayerToken: true},
		},
	},
	"web_music": {
		Name: "web_music", Host: "music.youtube.com",
		ClientName: "WEB_REMIX", ClientVersion: "1.20260707.12.00", ClientID: 67,
		SupportsCookies: true, RequireJSPlayer: true, GvsPolicies: webGvsPolicies(), Priority: 30,
	},
	"web_creator": {
		Name: "web_creator", ClientName: "WEB_CREATOR", ClientVersion: "1.20260708.06.00", ClientID: 62,
		SupportsCookies: true, RequireJSPlayer: true, GvsPolicies: webGvsPolicies(), Priority: 30,
	},
}

const (
	defaultClients = "visionos,web"
	apiProtocol    = "https"
	consentSOCS    = "CAI"
)

// innertubeContext builds the context JSON for a client.
func innertubeContext(c *Client, ytcfg map[string]any) map[string]any {
	client := map[string]any{
		"clientName":       c.ClientName,
		"clientVersion":    c.ClientVersion,
		"hl":               "en",
		"timeZone":         "UTC",
		"utcOffsetMinutes": 0,
	}
	for k, v := range c.ExtraClient {
		client[k] = v
	}
	// ytcfg override wins for these fields
	if ytcfg != nil {
		if ic, ok := ytcfg["INNERTUBE_CONTEXT"].(map[string]any); ok {
			if yc, ok := ic["client"].(map[string]any); ok {
				for _, k := range []string{"clientName", "clientVersion", "userAgent"} {
					if v, ok := yc[k]; ok {
						client[k] = v
					}
				}
			}
			// keep hl/timeZone forced
			client["hl"] = "en"
			client["timeZone"] = "UTC"
			client["utcOffsetMinutes"] = 0
		}
	}
	return map[string]any{"client": client}
}

// clientVersion resolves the effective client version (ytcfg wins).
func clientVersion(c *Client, ytcfg map[string]any) string {
	if ytcfg != nil {
		if v, ok := ytcfg["INNERTUBE_CLIENT_VERSION"].(string); ok && v != "" {
			return v
		}
		if ic, ok := ytcfg["INNERTUBE_CONTEXT"].(map[string]any); ok {
			if yc, ok := ic["client"].(map[string]any); ok {
				if v, ok := yc["clientVersion"].(string); ok && v != "" {
					return v
				}
			}
		}
	}
	return c.ClientVersion
}

// userAgent resolves the User-Agent header for a client ("" = omit).
func userAgent(c *Client, ytcfg map[string]any) string {
	if ytcfg != nil {
		if ic, ok := ytcfg["INNERTUBE_CONTEXT"].(map[string]any); ok {
			if yc, ok := ic["client"].(map[string]any); ok {
				if v, ok := yc["userAgent"].(string); ok && v != "" {
					return v
				}
			}
		}
	}
	return c.UserAgent
}

// apiHost selects the API hostname.
func apiHost(ie *Base) string {
	if v := ie.GetExtractorArg("innertube_host"); len(v) > 0 && v[0] != "" {
		return v[0]
	}
	return "www.youtube.com"
}

// callAPI POSTs to /youtubei/v1/<ep> and returns the parsed JSON.
func (ie *Base) callAPI(ep string, body map[string]any, c *Client, videoID string, ytcfg map[string]any) (map[string]any, error) {
	host := apiHost(ie)
	if c.Host != "" {
		host = c.Host
	}
	if v := ie.GetExtractorArg("innertube_host"); len(v) > 0 && v[0] != "" {
		host = v[0]
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"X-YouTube-Client-Name":    strconv.Itoa(c.ClientID),
		"X-YouTube-Client-Version": clientVersion(c, ytcfg),
		"Origin":                   apiProtocol + "://" + host,
		"Content-Type":             "application/json",
	}
	if ie.visitorData != "" {
		headers["X-Goog-Visitor-Id"] = ie.visitorData
	}
	if ua := userAgent(c, ytcfg); ua != "" {
		headers["User-Agent"] = ua
	}
	u := fmt.Sprintf("%s://%s/youtubei/v1/%s?prettyPrint=false", apiProtocol, host, ep)
	if key := ie.GetExtractorArg("innertube_key"); len(key) > 0 && key[0] != "" {
		u += "&key=" + key[0]
	}
	v, err := ie.DownloadJSON(extractor.NewRequest(u, payload, headers), videoID, "Downloading API JSON", "Unable to download API JSON", true)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	if m == nil {
		return nil, ie.NewExtractorError("Incomplete data received", false, nil)
	}
	return m, nil
}

// ytcfgRegex extracts ytcfg.set({...}; objects.
var ytcfgRegex = regexp.MustCompile(`ytcfg\.set\s*\(\s*({.+?})\s*\)\s*;`)

// downloadYtcfg fetches the ytcfg for a client ("" if unsupported/failed).
func (ie *Base) downloadYtcfg(c *Client, videoID string) map[string]any {
	var url string
	switch c.Name {
	case "mweb":
		url = "https://m.youtube.com"
	case "web", "web_safari":
		url = "https://www.youtube.com"
	case "web_music":
		url = "https://music.youtube.com"
	case "web_creator":
		url = "https://studio.youtube.com"
	case "web_embedded":
		url = fmt.Sprintf("https://www.youtube.com/embed/%s?html5=1", videoID)
	case "tv":
		url = "https://www.youtube.com/tv"
	default:
		return nil
	}
	if hasSkip(ie, "configs") {
		return nil
	}
	headers := map[string]string{}
	if c.UserAgent != "" {
		headers["User-Agent"] = c.UserAgent
	}
	page, err := ie.DownloadWebpage(extractor.NewRequest(url, nil, headers), videoID, "Downloading API configuration", "Unable to download API configuration", false, "")
	if err != nil || page == "" {
		return nil
	}
	m := ytcfgRegex.FindStringSubmatch(page)
	if m == nil {
		return nil
	}
	v, perr := extractor.ParseJSON(m[1], videoID, "ytcfg")
	if perr != nil {
		return nil
	}
	cfg, _ := v.(map[string]any)
	if cfg == nil {
		return nil
	}
	// fixes from _fix_embedded_ytcfg / tv anonymous config
	if c.Name == "web_embedded" {
		if ic, ok := cfg["INNERTUBE_CONTEXT"].(map[string]any); ok {
			ic["thirdParty"] = map[string]any{"embedUrl": "https://www.reddit.com/"}
		}
	}
	if c.Name == "tv" {
		if ic, ok := cfg["INNERTUBE_CONTEXT"].(map[string]any); ok {
			if cl, ok := ic["client"].(map[string]any); ok {
				if ci, ok := cl["configInfo"].(map[string]any); ok {
					delete(ci, "appInstallData")
				}
			}
		}
	}
	return cfg
}

// hasSkip reports whether the extractor-arg player_skip contains an item.
func hasSkip(ie interface{ GetExtractorArg(string) []string }, item string) bool {
	for _, s := range ie.GetExtractorArg("player_skip") {
		for _, part := range strings.Split(s, ",") {
			if strings.TrimSpace(part) == item {
				return true
			}
		}
	}
	return false
}

// extractVisitorData pulls visitorData from ytcfg or a player response.
func extractVisitorData(sources ...map[string]any) string {
	for _, s := range sources {
		if s == nil {
			continue
		}
		if v, ok := s["VISITOR_DATA"].(string); ok && v != "" {
			return v
		}
		if ic, ok := s["INNERTUBE_CONTEXT"].(map[string]any); ok {
			if cl, ok := ic["client"].(map[string]any); ok {
				if v, ok := cl["visitorData"].(string); ok && v != "" {
					return v
				}
			}
		}
		if rc, ok := s["responseContext"].(map[string]any); ok {
			if v, ok := rc["visitorData"].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// makeSIDAuthorization builds the SAPISIDHASH Authorization header value.
func makeSIDAuthorization(timestamp int64, sid, origin string, sessionID string) string {
	parts := []string{strconv.FormatInt(timestamp, 10), sid, origin}
	if sessionID != "" {
		parts = append(parts, ":u:"+sessionID)
	}
	msg := strings.Join(parts, " ")
	sum := sha1.Sum([]byte(msg))
	return "SAPISIDHASH " + strconv.FormatInt(timestamp, 10) + "_" + hex.EncodeToString(sum[:])
}

// cookieHeaders adds SAPISID auth headers when the jar has the right cookies
// (authentication is not supported in this milestone; returns nil).
func (ie *Base) cookieHeaders(client *Client, ytcfg map[string]any) map[string]any {
	return nil
}
