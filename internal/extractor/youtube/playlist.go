package youtube

import (
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
	regexp.MustCompile(`^https?://(?:[a-zA-Z0-9-]+\.)*(?:youtube\.com|youtube-nocookie\.com)/playlist\?(?:[^#&\n]*(?:&|))*list=(?P<id>[a-zA-Z0-9_-]{6,})`),
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

// RealExtract fetches the first page of the playlist via innertube browse.
func (ie *YoutubePlaylistIE) RealExtract(rawURL string) (*infodict.InfoDict, error) {
	u, _ := extractor.UnsmuggleURL(rawURL)
	playlistID := ie.MatchID(u)
	if playlistID == "" {
		return nil, ie.NewExtractorError("Unable to extract playlist ID", true, nil)
	}

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

	title := utils.TraverseStr(data, "metadata.playlistMetadataRenderer.title")
	if title == "" {
		title = playlistID
	}
	description := utils.TraverseStr(data, "metadata.playlistMetadataRenderer.description")
	return extractor.PlaylistResult(collectPlaylistEntries(data), playlistID, title, description), nil
}

// collectPlaylistEntries walks the first playback tab of a browse response and
// collects the playlistVideoRenderer / lockupViewModel entries.
func collectPlaylistEntries(data map[string]any) []*infodict.InfoDict {
	content := firstTabContent(data)
	if content == nil {
		return nil
	}
	var out []*infodict.InfoDict
	walkJSON(content, func(vm map[string]any) {
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
