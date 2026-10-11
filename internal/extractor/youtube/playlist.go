package youtube

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// YoutubePlaylistIE extracts YouTube playlists (first page of entries).
type YoutubePlaylistIE struct {
	Base
}

var playlistURLPatterns = []*regexp.Regexp{
	// /playlist?list=…, /watch?v=…&list=…, /embed/videoseries?list=…
	regexp.MustCompile(`^https?://(?:[a-zA-Z0-9-]+\.)*(?:youtube\.com|youtube-nocookie\.com)/(?:playlist|watch|embed/videoseries|videoseries)\?[^#\n]*\blist=(?P<id>[a-zA-Z0-9_-]{6,})`),
}

// PlaylistURLForURL returns the canonical URL that accesses the whole playlist
// referenced by any YouTube URL (watch, playlist or embed/videoseries) carrying
// a "list" query parameter. For example
// "https://www.youtube.com/watch?v=X&list=RDX&index=1" yields
// "https://www.youtube.com/playlist?list=RDX".
func (ie *YoutubePlaylistIE) PlaylistURLForURL(rawURL string) (string, error) {
	u, _ := extractor.UnsmuggleURL(rawURL)
	playlistID := ie.MatchID(u)
	if playlistID == "" {
		return "", ie.NewExtractorError("Unable to extract playlist ID", true, nil)
	}
	return "https://www.youtube.com/playlist?list=" + playlistID, nil
}

func init() {
	extractor.Register(NewYoutubePlaylistIE())
}

// NewYoutubePlaylistIE returns a fresh YouTube playlist extractor.
func NewYoutubePlaylistIE() *YoutubePlaylistIE {
	ie := &YoutubePlaylistIE{}
	ie.Name = "YoutubePlaylist"
	ie.Description = "YouTube playlists"
	ie.ValidURLRe = playlistURLPatterns
	ie.SetSelf(ie)
	return ie
}

// IEName returns the display name.
func (ie *YoutubePlaylistIE) IEName() string { return "YoutubePlaylist" }

// RealExtract fetches the first page of the playlist via innertube browse, or,
// for mix/radio playlists that have no dedicated playlist page, the contiguous
// queue embedded in the watch page.
func (ie *YoutubePlaylistIE) RealExtract(rawURL string) (*infodict.InfoDict, error) {
	playlistURL, err := ie.PlaylistURLForURL(rawURL)
	if err != nil {
		return nil, err
	}
	playlistID := ie.MatchID(playlistURL)
	if playlistID == "" {
		return nil, ie.NewExtractorError("Unable to extract playlist ID", true, nil)
	}
	ie.ToScreen("Playlist URL: %s", playlistURL)

	client := InnertubeClients["web"]
	ytcfg := ie.getYtcfg(client, playlistID)
	ie.visitorData = extractVisitorData(ytcfg)

	data, err := ie.callAPI("browse", map[string]any{
		"context":  innertubeContext(client, ytcfg),
		"browseId": "VL" + playlistID,
	}, client, playlistID, ytcfg)
	if err != nil {
		return nil, err
	}

	// Regular playlists (PL...) expose their first page in a browse tab.
	if c := firstTabContent(data); c != nil {
		title := utils.TraverseStr(data, "metadata.playlistMetadataRenderer.title")
		if title == "" {
			title = playlistID
		}
		description := utils.TraverseStr(data, "metadata.playlistMetadataRenderer.description")
		entries := collectPlaylistEntries(c)
		entries, err := ie.fetchPlaylistPages(entries, playlistID, client, ytcfg, playlistTabContinuationToken(c))
		if err != nil {
			return nil, err
		}
		return extractor.PlaylistResult(entries, playlistID, title, description), nil
	}

	// Mix/radio playlists (RD, AL, ...) are not browseable. Their content is
	// the contiguous queue of the video watch page.
	if isWatchURL(rawURL) {
		return ie.extractInlinePlaylist(rawURL, playlistID)
	}
	ie.ReportWarning("Unable to extract entries for playlist %s", playlistID)
	return extractor.PlaylistResult(nil, playlistID, playlistID, ""), nil
}

// isWatchURL reports whether the URL points at the video watch page.
func isWatchURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Path == "/watch"
}

// extractInlinePlaylist reads the playlist embedded in the video watch page
// (contents.…twoColumnWatchNextResults.playlist.playlist), which is how radio
// mixes are exposed. Falls back to the single video when no queue is present.
func (ie *YoutubePlaylistIE) extractInlinePlaylist(rawURL, fallbackID string) (*infodict.InfoDict, error) {
	headers := map[string]string{"User-Agent": InnertubeClients["web"].UserAgent}
	if headers["User-Agent"] == "" {
		headers = nil
	}
	q := map[string]string{"bpctr": "9999999999", "has_verified": "1"}
	req := extractor.NewRequest(rawURL, nil, headers).WithQuery(q)
	webpage, err := ie.DownloadWebpage(req, fallbackID, "", "Unable to download webpage", false, "")
	if err != nil {
		return nil, err
	}
	data := extractInitialJSON(reInitialData, webpage)
	if data == nil {
		return nil, ie.NewExtractorError("Unable to extract playlist from watch page", true, nil)
	}
	playlist := utils.TraverseMap(data, "contents.oneClickYtWatchPageResults.twoColumnWatchNextResults.playlist.playlist")
	if playlist == nil {
		playlist = utils.TraverseMap(data, "contents.twoColumnWatchNextResults.playlist.playlist")
	}
	if playlist == nil {
		ie.ReportWarning("Unable to recognize playlist. Downloading just the video")
		if videoID := videoIDFromURL(rawURL); videoID != "" {
			return extractor.URLResult("https://www.youtube.com/watch?v="+videoID, "Youtube", videoID, ""), nil
		}
		return nil, ie.NewExtractorError("Unable to extract playlist", true, nil)
	}
	id := utils.Str(playlist["playlistId"])
	if id == "" {
		id = fallbackID
	}
	title := textOf(playlist["title"])
	if title == "" {
		if t := utils.TraverseMap(playlist, "titleText"); t != nil {
			title = textOf(t)
		}
	}
	return extractor.PlaylistResult(collectInlinePlaylistEntries(playlist), id, title, ""), nil
}

// videoIDFromURL returns the "v" query parameter of a YouTube URL.
func videoIDFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("v")
}

// collectInlinePlaylistEntries walks the contents of an inline
// (twoColumnWatchNextResults) playlist and collects the video entries.
func collectInlinePlaylistEntries(playlist map[string]any) []*infodict.InfoDict {
	var out []*infodict.InfoDict
	walkJSON(playlist["contents"], func(vm map[string]any) {
		for _, key := range []string{"playlistVideoRenderer", "playlistPanelVideoRenderer"} {
			renderer, ok := vm[key].(map[string]any)
			if !ok {
				continue
			}
			id := utils.Str(renderer["videoId"])
			if id == "" {
				return
			}
			length := ""
			if lt := utils.TraverseMap(renderer, "lengthText"); lt != nil {
				length = textOf(lt)
			}
			out = append(out, playlistEntry(id, textOf(renderer["title"]), length))
			return
		}
	})
	return out
}

// fetchPlaylistPages appends the pages reachable through the browse
// continuation token until the playlist is exhausted or maxEntries entries have
// been collected (0 = no cap).
func (ie *YoutubePlaylistIE) fetchPlaylistPages(entries []*infodict.InfoDict, playlistID string, client *Client, ytcfg map[string]any, token string) ([]*infodict.InfoDict, error) {
	maxEntries := ie.PlaylistMaxEntries()
	for token != "" && (maxEntries == 0 || len(entries) < maxEntries) {
		next, err := ie.callAPI("browse", map[string]any{
			"context":      innertubeContext(client, ytcfg),
			"continuation": token,
		}, client, playlistID, ytcfg)
		if err != nil {
			if maxEntries == 0 || len(entries) == 0 {
				return nil, err
			}
			ie.ReportWarning("Unable to download more playlist pages: %v", err)
			break
		}
		entries = append(entries, collectVideoEntries(continuationItems(next))...)
		token = continuationResponseToken(next)
	}
	if maxEntries > 0 && len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}
	return entries, nil
}

// playlistTabContinuationToken extracts the "load more" continuation token of
// the first tab of a playlist browse page, if any.
func playlistTabContinuationToken(content map[string]any) string {
	if tok := findInside(content, "playlistVideoListRenderer"); tok != "" {
		return tok
	}
	return findContinuationToken(content)
}

// findInside returns the first continuation token found in the subtree rooted
// at the first occurrence of the given key anywhere in v.
func findInside(v any, key string) string {
	sub := findSubtree(v, key)
	if sub == nil {
		return ""
	}
	return findContinuationToken(sub)
}

func findSubtree(v any, key string) any {
	switch t := v.(type) {
	case map[string]any:
		if sub, ok := t[key]; ok {
			return sub
		}
		for _, c := range t {
			if sub := findSubtree(c, key); sub != nil {
				return sub
			}
		}
	case []any:
		for _, c := range t {
			if sub := findSubtree(c, key); sub != nil {
				return sub
			}
		}
	}
	return nil
}

// continuationResponseToken extracts the continuation token of a browse
// continuation response.
func continuationResponseToken(data map[string]any) string {
	for _, item := range list(data["onResponseReceivedActions"]) {
		m, _ := item.(map[string]any)
		if tok := findContinuationToken(appendContinuationItems(m)); tok != "" {
			return tok
		}
	}
	return ""
}

// findContinuationToken scans a JSON subtree for the first continuationCommand
// carrying a token. This matches all current renderer variants
// (continuationItemRenderer, continuationItemViewModel, endpoint commands...).
func findContinuationToken(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if cc, ok := t["continuationCommand"].(map[string]any); ok {
			if tok, ok := cc["token"].(string); ok && tok != "" {
				return tok
			}
		}
		for _, c := range t {
			if tok := findContinuationToken(c); tok != "" {
				return tok
			}
		}
	case []any:
		for _, c := range t {
			if tok := findContinuationToken(c); tok != "" {
				return tok
			}
		}
	}
	return ""
}

// list returns v as a slice, or nil.
func list(v any) []any {
	l, _ := v.([]any)
	return l
}

// appendContinuationItems returns the continuationItems subtree of an
// appendContinuationItemsAction / reloadContinuationItemsCommand.
func appendContinuationItems(m map[string]any) any {
	if act := utils.TraverseMap(m, "appendContinuationItemsAction"); act != nil {
		return act["continuationItems"]
	}
	if act := utils.TraverseMap(m, "reloadContinuationItemsCommand"); act != nil {
		return act["continuationItems"]
	}
	return nil
}

// continuationItems walks to the continuationItems of the response root.
func continuationItems(data map[string]any) any {
	var out []any
	for _, item := range list(data["onResponseReceivedActions"]) {
		m, _ := item.(map[string]any)
		if v := appendContinuationItems(m); v != nil {
			if l, ok := v.([]any); ok {
				out = append(out, l...)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectVideoEntries walks a JSON tree and collects the playlist video
// entries (playlistVideoRenderer / lockupViewModel).
func collectVideoEntries(v any) []*infodict.InfoDict {
	var out []*infodict.InfoDict
	walkJSON(v, func(vm map[string]any) {
		if pv, ok := vm["playlistVideoRenderer"].(map[string]any); ok {
			if id := utils.Str(pv["videoId"]); id != "" {
				out = append(out, playlistEntry(id, textOf(pv["title"]), utils.Str(pv["lengthSeconds"])))
			}
			return
		}
		if lv, ok := vm["lockupViewModel"].(map[string]any); ok {
			if strings.Contains(utils.Str(lv["contentType"]), "LOCKUP_CONTENT_TYPE_VIDEO") {
				if id := utils.Str(lv["contentId"]); id != "" {
					out = append(out, playlistEntry(id, utils.Str(utils.Traverse(lv, "metadata.lockupMetadataViewModel.title.content")), ""))
				}
			}
		}
	})
	return out
}

// collectPlaylistEntries walks the first playback tab of a browse response and
// collects the playlistVideoRenderer / lockupViewModel entries.
func collectPlaylistEntries(content map[string]any) []*infodict.InfoDict {
	return collectVideoEntries(content)
}

// playlistEntry builds the redirect result for a playlist video.
func playlistEntry(videoID, title, durationStr string) *infodict.InfoDict {
	entry := extractor.URLResult("https://www.youtube.com/watch?v="+videoID, "Youtube", videoID, title)
	if n, err := strconv.Atoi(durationStr); err == nil && n > 0 {
		entry.Duration = float64(n)
	}
	return entry
}

// firstTabContent returns the "content" subtree of the first tab renderer found
// in a twoColumnBrowseResultsRenderer (the active tab of a playlist page).
func firstTabContent(data map[string]any) map[string]any {
	browse := utils.TraverseMap(data, "contents.twoColumnBrowseResultsRenderer")
	if browse == nil {
		return nil
	}
	tabs, _ := browse["tabs"].([]any)
	for _, t := range tabs {
		tm, _ := t.(map[string]any)
		tab := utils.TraverseMap(tm, "tabRenderer")
		if tab == nil {
			tab = utils.TraverseMap(tm, "expandableTabRenderer")
		}
		if tab == nil {
			continue
		}
		if c, ok := tab["content"].(map[string]any); ok {
			return c
		}
	}
	return nil
}

// walkJSON visits every map in the JSON tree rooted at v.
func walkJSON(v any, visit func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		visit(t)
		for _, c := range t {
			walkJSON(c, visit)
		}
	case []any:
		for _, c := range t {
			walkJSON(c, visit)
		}
	}
}
