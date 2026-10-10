package youtube

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// YoutubeIE is the main YouTube video extractor.
type YoutubeIE struct {
	Base
}

func init() {
	extractor.Register(NewYoutubeIE())
}

// NewYoutubeIE returns a fresh, ready-to-use YouTube extractor.
func NewYoutubeIE() *YoutubeIE {
	ie := &YoutubeIE{}
	ie.Name = "Youtube"
	ie.Description = "YouTube videos"
	ie.ValidURLRe = validURLPatterns()
	ie.SetSelf(ie)
	return ie
}

// Suitable rejects /v/videoseries and /live/live_stream pseudo-IDs.
func (ie *YoutubeIE) Suitable(u string) bool {
	return ie.Base.Suitable(u) && !reNonVideoPath.MatchString(u)
}

// IEName returns the display name.
func (ie *YoutubeIE) IEName() string { return "Youtube" }

// RealExtract is the main extraction entry point.
func (ie *YoutubeIE) RealExtract(rawURL string) (*infodict.InfoDict, error) {
	url, smuggled := extractor.UnsmuggleURL(rawURL)
	videoID := ie.MatchID(url)
	if videoID == "" {
		return nil, ie.NewExtractorError("Unable to extract video ID", true, nil)
	}
	_ = smuggled
	webpageURL := "https://www.youtube.com/watch?v=" + videoID

	// 1. Webpage --------------------------------------------------------
	webpage := ""
	if !hasSkip(ie, "webpage") {
		headers := map[string]string{"User-Agent": InnertubeClients["web"].UserAgent}
		if headers["User-Agent"] == "" {
			headers = nil
		}
		q := map[string]string{"bpctr": "9999999999", "has_verified": "1"}
		req := extractor.NewRequest(webpageURL, nil, headers).WithQuery(q)
		w, err := ie.DownloadWebpage(req, videoID, "", "Unable to download webpage", false, "")
		if err != nil {
			return nil, err
		}
		webpage = w
	}
	webpageYTCfg := extractYTCfg(webpage)
	initialData := extractInitialJSON(reInitialData, webpage)
	initialPR := extractInitialJSON(reInitialPR, webpage)
	if initialData == nil && !hasSkip(ie, "initial_data") {
		initialData = ie.downloadInitialData(videoID, webpageYTCfg)
	}

	// 2. Player responses ----------------------------------------------
	clients, err := ie.requestedClients(false)
	if err != nil {
		return nil, err
	}
	ie.playerURL = ie.extractPlayerURL(webpageYTCfg)

	prs, playerURL, err := ie.extractPlayerResponses(clients, videoID, webpage, initialPR, webpageYTCfg)
	if err != nil {
		return nil, err
	}
	ie.playerURL = playerURL

	// 3. Metadata collections ------------------------------------------
	var videoDetails, microformats []map[string]any
	for _, pr := range prs {
		if vd, ok := pr["videoDetails"].(map[string]any); ok {
			videoDetails = append(videoDetails, vd)
		}
		if mf := utils.TraverseMap(pr, "microformat.playerMicroformatRenderer"); mf != nil {
			microformats = append(microformats, mf)
		}
	}

	// Trailer redirect (age-restricted with promotion)
	for _, pr := range prs {
		if tid := utils.TraverseStr(pr, "playabilityStatus.errorScreen.playerLegacyDesktopYpcTrailerRenderer.trailerVideoId"); tid != "" {
			return extractor.URLResult(tid, "Youtube", tid, ""), nil
		}
	}

	// 4. Duration / live status ----------------------------------------
	duration := firstNonZero(
		firstFloat(videoDetails, "lengthSeconds"),
		firstFloat(microformats, "lengthSeconds"),
	)
	liveStatus := computeLiveStatus(videoDetails, microformats)

	// 5. Formats & subtitles -------------------------------------------
	var formats []infodict.Format
	var subs, autoCaps map[string][]infodict.Subtitle
	if !hasSkip(ie, "player_response") {
		formats, subs, autoCaps, err = ie.extractFormatsAndSubtitles(prs, videoID, duration, liveStatus)
		if err != nil {
			return nil, err
		}
	}

	if len(formats) == 0 {
		if e := ie.noFormatsError(prs, microformats, webpage); e != nil {
			return nil, e
		}
	}

	// 6. Title / description -------------------------------------------
	meta := &extractMeta{videoDetails: videoDetails, microformats: microformats}
	title := ie.extractTitle(meta, initialData, webpage)
	description := ie.extractDescription(meta, initialData)

	// 7. Assemble InfoDict ---------------------------------------------
	info := &infodict.InfoDict{
		ID:                videoID,
		DisplayID:         videoID,
		Title:             title,
		Description:       description,
		Duration:          duration,
		Formats:           formats,
		Subtitles:         subs,
		AutomaticCaptions: autoCaps,
		WebpageURL:        webpageURL,
		OriginalURL:       rawURL,
		Extractor:         ie.IEName(),
		ExtractorKey:      ie.IEKey(),
	}
	if duration > 0 {
		info.DurationString = utils.FormatDurationString(duration)
	}
	ie.fillMetadata(info, meta, initialData, webpage, liveStatus)
	for _, pr := range prs {
		if v, ok := utils.Traverse(pr, "playabilityStatus.playableInEmbed").(bool); ok {
			info.PlayableInEmbed = &v
			break
		}
	}
	ie.extractThumbnails(info, meta, webpage, videoID)
	ie.extractChaptersAndHeatmap(info, initialData, webpage)

	return info, nil
}

// ---------------------------------------------------------------------------
// Player responses
// ---------------------------------------------------------------------------

func (ie *YoutubeIE) extractPlayerResponses(
	clients []string, videoID, webpage string,
	initialPR map[string]any, webpageYTCfg map[string]any,
) ([]map[string]any, string, error) {
	var prs []map[string]any
	if initialPR != nil && validPlayerResponse(initialPR, videoID) {
		// metadata-only copy for the initial pass
		cp := copyMap(initialPR)
		delete(cp, "streamingData")
		prs = append(prs, cp)
	}

	playerURL := ie.playerURL
	queue := append([]string{}, clients...)
	skipped := map[string]string{}
	ie.visitorData = extractVisitorData(webpageYTCfg, initialPR)

	for len(queue) > 0 {
		clientName := queue[0]
		queue = queue[1:]
		client := InnertubeClients[clientName]
		if client == nil {
			ie.ReportWarning("Unknown player client: %s", clientName)
			continue
		}
		baseName, variant := func() (string, string) {
			c, b, v := splitInnertubeClient(clientName)
			if c != clientName {
				client = InnertubeClients[c]
			}
			return b, v
		}()
		if client == nil {
			ie.ReportWarning("Unknown player client: %s", clientName)
			continue
		}
		ytcfg := ie.getYtcfg(client, videoID)
		if ie.visitorData == "" {
			ie.visitorData = extractVisitorData(ytcfg)
		}
		if playerURL == "" {
			playerURL = ie.extractPlayerURL(webpageYTCfg, ytcfg)
		}
		if playerURL != "" && ie.playerJS == "" && client.RequireJSPlayer {
			_ = ie.loadPlayer(videoID)
		}

		var pr map[string]any
		if clientName == "web" && initialPR != nil {
			pr = initialPR
		} else {
			var perr error
			pr, perr = ie.extractPlayerResponse(client, videoID, ytcfg)
			if perr != nil {
				ie.ReportWarning("%v", perr)
				continue
			}
		}
		if pr == nil {
			continue
		}
		if !validPlayerResponse(pr, videoID) {
			skipped[clientName] = utils.TraverseStr(pr, "videoDetails.videoId")
			continue
		}
		// annotate streamingData with the client name
		if sd, ok := pr["streamingData"].(map[string]any); ok {
			if _, has := sd["__yt_dlp_client"]; !has {
				cp := copyMap(pr)
				sdcp := copyMap(sd)
				sdcp["__yt_dlp_client"] = clientName
				cp["streamingData"] = sdcp
				pr = cp
			}
		}
		prs = append(prs, pr)

		status := utils.TraverseStr(pr, "playabilityStatus.status")
		// dynamic fallbacks
		if (clientName == "android_vr" || clientName == "visionos") &&
			(status == "UNPLAYABLE" || status == "ERROR") {
			queue = append(queue, "web_embedded", "tv_downgraded")
		}
		if isAgeGated(pr) && variant != "embedded" && baseName != "" {
			queue = append(queue, "web_embedded."+baseName)
		}
	}
	if len(skipped) > 0 {
		var parts []string
		for k, v := range skipped {
			parts = append(parts, fmt.Sprintf("%s (video id: %s)", k, v))
		}
		ie.ReportWarning("Skipped player clients: %s", strings.Join(parts, ", "))
	}
	_ = playerURL
	if len(prs) == 0 {
		return nil, playerURL, ie.NewExtractorError(
			"All player responses are invalid. Your IP is likely being blocked by Youtube", false, nil)
	}
	return prs, playerURL, nil
}

func (ie *YoutubeIE) extractPlayerResponse(c *Client, videoID string, ytcfg map[string]any) (map[string]any, error) {
	body := map[string]any{
		"context":        innertubeContext(c, ytcfg),
		"videoId":        videoID,
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
	playback := map[string]any{
		"contentPlaybackContext": map[string]any{
			"html5Preference": "HTML5_PREF_WANTS",
		},
	}
	if ie.playerJS != "" {
		if sts := ie.signatureTimestamp(); sts > 0 {
			playback["contentPlaybackContext"].(map[string]any)["signatureTimestamp"] = sts
		}
	}
	body["playbackContext"] = playback
	if pp := ie.GetExtractorArg("player_params"); len(pp) > 0 && pp[0] != "" {
		body["params"] = pp[0]
	}
	return ie.callAPI("player", body, c, videoID, ytcfg)
}

func validPlayerResponse(pr map[string]any, videoID string) bool {
	if pr == nil {
		return false
	}
	return utils.TraverseStr(pr, "videoDetails.videoId") == videoID
}

func isAgeGated(pr map[string]any) bool {
	if utils.TraverseStr(pr, "playabilityStatus.desktopLegacyAgeGateReason") != "" {
		return true
	}
	for _, field := range []string{"playabilityStatus.status", "playabilityStatus.reason"} {
		v := utils.TraverseStr(pr, field)
		for _, reason := range []string{"confirm your age", "age-restricted", "inappropriate",
			"age_verification_required", "age_check_required"} {
			if strings.Contains(v, reason) {
				return true
			}
		}
	}
	return false
}

func (ie *YoutubeIE) downloadInitialData(videoID string, webpageYTCfg map[string]any) map[string]any {
	body := map[string]any{
		"context": innertubeContext(InnertubeClients["web"], webpageYTCfg),
		"videoId": videoID,
	}
	v, err := ie.callAPI("next", body, InnertubeClients["web"], videoID, webpageYTCfg)
	if err != nil {
		return nil
	}
	return v
}

// ---------------------------------------------------------------------------
// Metadata helpers
// ---------------------------------------------------------------------------

type extractMeta struct {
	videoDetails []map[string]any
	microformats []map[string]any
}

func firstStr(maps []map[string]any, key string) string {
	for _, m := range maps {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func firstAny(maps []map[string]any, key string) any {
	for _, m := range maps {
		if v := m[key]; v != nil {
			return v
		}
	}
	return nil
}

func firstFloat(maps []map[string]any, key string) float64 {
	for _, m := range maps {
		if v := m[key]; v != nil {
			if f, ok := utils.Float64(v); ok {
				return f
			}
		}
	}
	return 0
}

func firstNonZero(vals ...float64) float64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func computeLiveStatus(videoDetails, microformats []map[string]any) string {
	isLive := firstAny(videoDetails, "isLive")
	if isLive == nil {
		isLive = firstAny(microformats, "liveBroadcastDetails.isLiveNow")
	}
	isLiveContent := firstAny(videoDetails, "isLiveContent")
	isUpcoming := firstAny(videoDetails, "isUpcoming")
	postLive := firstAny(videoDetails, "isPostLiveDvr")
	if postLive == true {
		return "post_live"
	}
	if isLive == true {
		return "is_live"
	}
	if isUpcoming == true {
		return "is_upcoming"
	}
	if isLiveContent == true {
		return "was_live"
	}
	if isLive == false && isLiveContent == false {
		return "not_live"
	}
	return ""
}

func (ie *YoutubeIE) extractTitle(meta *extractMeta, initialData map[string]any, webpage string) string {
	if t := firstStr(meta.microformats, "title"); t != "" {
		return t
	}
	if t := firstStr(meta.videoDetails, "title"); t != "" {
		return t
	}
	if webpage != "" {
		for _, k := range []string{"og:title", "twitter:title", "title"} {
			if t := ie.HTMLSearchMeta(k, webpage); t != "" {
				return t
			}
		}
	}
	if t := initialTitle(initialData); t != "" {
		ie.ReportWarning("No title found in player responses; falling back to title from initial data. Other metadata may also be missing")
		return t
	}
	return ""
}

func initialTitle(initialData map[string]any) string {
	if initialData == nil {
		return ""
	}
	// playerOverlays...playerOverlayVideoDetailsRenderer.title
	if t := textOf(utils.Traverse(initialData,
		"playerOverlays.playerOverlayRenderer.videoDetails.playerOverlayVideoDetailsRenderer.title")); t != "" {
		return t
	}
	// engagementPanels ... videoDescriptionHeaderRenderer.title
	for _, p := range asList(initialData["engagementPanels"]) {
		pm, _ := p.(map[string]any)
		if pm == nil {
			continue
		}
		items := asList(utils.Traverse(pm,
			"engagementPanelSectionListRenderer.content.structuredDescriptionContentRenderer.items"))
		for _, it := range items {
			im, _ := it.(map[string]any)
			if im == nil {
				continue
			}
			if vdhr, ok := im["videoDescriptionHeaderRenderer"].(map[string]any); ok {
				if t := textOf(vdhr["title"]); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func (ie *YoutubeIE) extractDescription(meta *extractMeta, initialData map[string]any) string {
	if d := firstAny(meta.videoDetails, "shortDescription"); d != nil {
		if s, ok := d.(string); ok {
			return s
		}
	}
	if d := firstStr(meta.microformats, "description"); d != "" {
		return d
	}
	if initialData == nil {
		return ""
	}
	for _, p := range asList(initialData["engagementPanels"]) {
		pm, _ := p.(map[string]any)
		if pm == nil {
			continue
		}
		items := asList(utils.Traverse(pm,
			"engagementPanelSectionListRenderer.content.structuredDescriptionContentRenderer.items"))
		for _, it := range items {
			im, _ := it.(map[string]any)
			if im == nil {
				continue
			}
			if body := utils.Traverse(im, "expandableVideoDescriptionBodyRenderer.attributedDescriptionBodyText.content"); body != nil {
				if s, ok := body.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func (ie *YoutubeIE) fillMetadata(info *infodict.InfoDict, meta *extractMeta, initialData map[string]any, webpage, liveStatus string) {
	// channel / uploader
	info.ChannelID = firstStr(meta.videoDetails, "channelId")
	if info.ChannelID == "" {
		info.ChannelID = firstStr(meta.microformats, "externalChannelId")
	}
	if info.ChannelID != "" {
		info.ChannelURL = "https://www.youtube.com/channel/" + info.ChannelID
	}
	if u := utils.TraverseStr(initialData,
		"contents.twoColumnWatchNextResults.results.results.contents.videoSecondaryInfoRenderer.owner.videoOwnerRenderer.title"); u != "" {
		info.Uploader = u
		info.Channel = u
	} else if vd := firstStr(meta.videoDetails, "author"); vd != "" {
		info.Uploader = vd
		info.Channel = vd
	}
	info.UploaderID = strings.TrimPrefix(firstStr(meta.microformats, "ownerProfileUrl"),
		"https://www.youtube.com/")
	if info.UploaderID != "" {
		info.UploaderURL = "https://www.youtube.com/" + info.UploaderID
	}

	// dates
	if iso := firstStr(meta.microformats, "uploadDate"); iso != "" {
		if ts := utils.ParseISO8601(iso); ts != nil {
			info.Timestamp = ts
			info.UploadDate = utils.UnixToYYYYMMDD(*ts)
		}
	}
	if ts := utils.ParseISO8601(firstStr(meta.microformats, "publishDate")); ts != nil && info.Timestamp == nil {
		info.Timestamp = ts
		info.UploadDate = utils.UnixToYYYYMMDD(*ts)
	}

	// counts
	if v := firstAny(meta.videoDetails, "viewCount"); v != nil {
		info.ViewCount = utils.IntOrNone(v)
	}
	if info.ViewCount == nil {
		if m := ie.HTMLSearchMeta("interactionCount", webpage); m != "" {
			info.ViewCount = utils.StrToInt(m)
		}
	}
	info.LikeCount = utils.ParseCount(utils.TraverseStr(initialData,
		"contents.twoColumnWatchNextResults.results.results.contents.videoPrimaryInfoRenderer.videoActions.menuRenderer.topLevelButtons.segmentedLikeDislikeButtonViewModel.likeButtonViewModel.likeButtonViewModel.likeButton.accessibilityText"))

	// category / tags / age limit
	if c := firstStr(meta.microformats, "category"); c != "" {
		info.Categories = []string{c}
	}
	if kw := firstAny(meta.videoDetails, "keywords"); kw != nil {
		for _, k := range asList(kw) {
			if s, ok := k.(string); ok && s != "" {
				info.Tags = append(info.Tags, s)
			}
		}
	}
	age := 0
	if firstAny(meta.microformats, "isFamilySafe") == false {
		age = 18
	} else if strings.EqualFold(ie.HTMLSearchMeta("isFamilyFriendly", webpage), "false") ||
		ie.HTMLSearchMeta("og:restrictions:age", webpage) == "18+" {
		age = 18
	}
	if age > 0 {
		info.AgeLimit = &age
	}

	// live status
	info.LiveStatus = liveStatus
	isLive := liveStatus == "is_live"
	info.IsLive = &isLive

	// comments
	if cc := utils.ParseCount(utils.TraverseStr(initialData,
		"contents.twoColumnWatchNextResults.results.results.contents.commentsEntryPointHeaderRenderer.commentCount")); cc != nil {
		info.CommentCount = cc
	}
}

func (ie *YoutubeIE) extractThumbnails(info *infodict.InfoDict, meta *extractMeta, webpage, videoID string) {
	var thumbs []infodict.Thumbnail
	for _, m := range append(append([]map[string]any{}, meta.videoDetails...), meta.microformats...) {
		for _, t := range asList(utils.Traverse(m, "thumbnail.thumbnails")) {
			tm, _ := t.(map[string]any)
			if tm == nil {
				continue
			}
			u := utils.Str(tm["url"])
			if u == "" {
				continue
			}
			thumbs = append(thumbs, infodict.Thumbnail{
				URL:    u,
				Width:  utils.SafeInt(tm["width"]),
				Height: utils.SafeInt(tm["height"]),
			})
		}
	}
	if len(thumbs) == 0 {
		if u := ie.OGSearchThumbnail(webpage); u != "" {
			thumbs = append(thumbs, infodict.Thumbnail{URL: u})
		}
	}
	// guessed URLs
	names := []string{"maxresdefault", "hq720", "sddefault", "hqdefault", "0",
		"mqdefault", "default", "sd1", "sd2", "sd3", "hq1", "hq2", "hq3", "mq1", "mq2", "mq3", "1", "2", "3"}
	idx := 0
	for _, ext := range []string{"webp", "jpg"} {
		for _, n := range names {
			pref := -1 - 2*idx
			if ext == "webp" {
				pref = -2 * idx
			}
			u := fmt.Sprintf("https://i.ytimg.com/vi/%s/%s.%s", videoID, n, ext)
			thumbs = append(thumbs, infodict.Thumbnail{
				ID:         n,
				URL:        u,
				Ext:        ext,
				Preference: ip(int64(pref)),
			})
			idx++
		}
	}
	info.Thumbnails = thumbs
	// best thumbnail
	best := ""
	bestPref := -1 << 30
	for _, t := range thumbs {
		p := -1 << 30
		if t.Preference != nil {
			p = *t.Preference
		}
		if p > bestPref {
			bestPref = p
			best = t.URL
		}
	}
	info.Thumbnail = best
}

func (ie *YoutubeIE) extractChaptersAndHeatmap(info *infodict.InfoDict, initialData map[string]any, webpage string) {
	if initialData == nil {
		return
	}
	// chapters from chapteredPlayerBarRenderer
	var chapters []infodict.Chapter
	for _, ch := range asList(utils.Traverse(initialData,
		"playerOverlays.playerOverlayRenderer.decoratedPlayerBarRenderer.playerBar.chapteredPlayerBarRenderer.chapters")) {
		cm, _ := ch.(map[string]any)
		if cm == nil {
			continue
		}
		cr, _ := cm["chapterRenderer"].(map[string]any)
		if cr == nil {
			continue
		}
		start := 0.0
		if ms := utils.IntOrNone(cr["startMillis"]); ms != nil {
			start = float64(*ms) / 1000
		}
		chapters = append(chapters, infodict.Chapter{
			StartTime: start,
			Title:     textOf(cr["title"]),
		})
	}
	if len(chapters) > 1 {
		for i := range chapters {
			if i+1 < len(chapters) {
				e := chapters[i+1].StartTime
				chapters[i].EndTime = &e
			}
		}
		info.Chapters = chapters
	}
	// heatmap
	for _, mut := range asList(utils.Traverse(initialData, "frameworkUpdates.entityBatchUpdate.mutations")) {
		mm, _ := mut.(map[string]any)
		if mm == nil {
			continue
		}
		markers := utils.Traverse(mm, "payload.macroMarkersListEntity.markersList.markers")
		for _, mk := range asList(markers) {
			km, _ := mk.(map[string]any)
			if km == nil {
				continue
			}
			var start, dur float64
			if v := utils.IntOrNone(km["startMillis"]); v != nil {
				start = float64(*v) / 1000
			}
			if v := utils.IntOrNone(km["durationMillis"]); v != nil {
				dur = float64(*v) / 1000
			}
			info.Heatmap = append(info.Heatmap, infodict.HeatmapSegment{
				StartTime: start,
				EndTime:   start + dur,
				Value:     f64d(utils.FloatOrNone(km["intensityScoreNormalized"])),
			})
		}
	}
}

// noFormatsError builds the proper error when no formats were found.
func (ie *YoutubeIE) noFormatsError(prs []map[string]any, microformats []map[string]any, webpage string) error {
	var statuses []map[string]any
	for _, pr := range prs {
		if ps, ok := pr["playabilityStatus"].(map[string]any); ok {
			statuses = append(statuses, ps)
		}
	}
	reason := ""
	for _, s := range statuses {
		if r := textOf(utils.Traverse(s, "errorScreen.playerErrorMessageRenderer.reason")); r != "" {
			reason = r
			break
		}
	}
	if reason == "" {
		for _, s := range statuses {
			if r := textOf(s["reason"]); r != "" {
				reason = r
				break
			}
		}
	}
	subreason := ""
	for _, s := range statuses {
		if r := utils.CleanHTML(textOf(utils.Traverse(s, "errorScreen.playerErrorMessageRenderer.subreason"))); r != "" {
			subreason = r
			break
		}
	}
	if subreason != "" {
		if strings.HasPrefix(subreason, "The uploader has not made this video available in your country") {
			countries := asList(firstAny(microformats, "availableCountries"))
			var cc []string
			for _, c := range countries {
				cc = append(cc, utils.Str(c))
			}
			if len(cc) == 0 {
				if ra := ie.HTMLSearchMeta("regionsAllowed", webpage); ra != "" {
					cc = strings.Split(ra, ",")
				}
			}
			return ie.RaiseGeoRestricted(subreason, cc)
		}
		reason += ". " + subreason
	}
	if reason == "" {
		reason = "Requested format is not available"
	}
	return ie.RaiseNoFormats(reason, true)
}

// ---------------------------------------------------------------------------
// misc
// ---------------------------------------------------------------------------

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

var reYTCfgSet = regexp.MustCompile(`ytcfg\.set\s*\(\s*({.+?})\s*\)\s*;`)

func extractYTCfg(page string) map[string]any {
	if page == "" {
		return nil
	}
	m := reYTCfgSet.FindStringSubmatch(page)
	if m == nil {
		return nil
	}
	v, err := extractor.ParseJSON(m[1], "", "ytcfg")
	if err != nil {
		return nil
	}
	cfg, _ := v.(map[string]any)
	return cfg
}
