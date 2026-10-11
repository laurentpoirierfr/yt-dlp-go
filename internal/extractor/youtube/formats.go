package youtube

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/jsc"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

type streamID struct {
	itag  string
	track string
	isDrc bool
}

// resQuality is one entry of yt-dlp's `res_qualities` mapping.
type resQuality struct {
	height  int
	quality string
}

// formatState carries the cross-pass bookkeeping of
// _extract_formats_and_subtitles: the itag -> {(proto, language)} set, the
// itag/height -> quality maps and the original/default language map.
type formatState struct {
	itags         map[string]map[string]bool
	itagQualities map[string]string
	resQualities  []resQuality
	languageMap   map[int]string
	allFormats    bool
	liveStatus    string
}

func newFormatState(liveStatus string) *formatState {
	return &formatState{
		itags:         map[string]map[string]bool{},
		itagQualities: map[string]string{},
		resQualities:  []resQuality{{0, ""}},
		languageMap:   map[int]string{10: "", 5: ""},
		liveStatus:    liveStatus,
	}
}

// setResQuality records (or updates) the quality for a resolution height.
func (st *formatState) setResQuality(height int, quality string) {
	for i := range st.resQualities {
		if st.resQualities[i].height == height {
			st.resQualities[i].quality = quality
			return
		}
	}
	st.resQualities = append(st.resQualities, resQuality{height, quality})
}

// nearestResQuality returns the quality of the closest known height (ties keep
// the earliest inserted entry, like Python's min()).
func (st *formatState) nearestResQuality(height int) (string, bool) {
	if len(st.resQualities) == 0 {
		return "", false
	}
	best := 0
	bestDiff := absInt(st.resQualities[0].height - height)
	for i := 1; i < len(st.resQualities); i++ {
		d := absInt(st.resQualities[i].height - height)
		if d < bestDiff {
			best, bestDiff = i, d
		}
	}
	return st.resQualities[best].quality, true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var reManifestN = regexp.MustCompile(`/n/([^/]+)/`)
var reMimeType = regexp.MustCompile(`((?:[^/]+)/(?:[^;]+))(?:;\s*codecs="([^"]+)")?`)

// gvsPotRequired implements yt-dlp's gvs_pot_required.
func gvsPotRequired(p GvsPoTokenPolicy, isPremium, hasPlayerToken bool) bool {
	return p.Required &&
		!(p.NotRequiredWithPlayerToken && hasPlayerToken) &&
		!(p.NotRequiredForPremium && isPremium)
}

func (c *Client) gvsPolicy(p StreamingProtocol) GvsPoTokenPolicy {
	return c.GvsPolicies[p] // zero value = nothing required
}

// parseQueryString parses a query string into map[string][]string.
func parseQueryString(raw string) map[string][]string {
	m, _ := url.ParseQuery(raw)
	return m
}

// extractFormatsAndSubtitles is the port of _extract_formats_and_subtitles.
// HTTPS formats are always extracted; for each player response its HLS
// manifest (when present) is parsed too (DASH manifests are not yet handled).
func (b *Base) extractFormatsAndSubtitles(
	prs []map[string]any, videoID string, duration float64, liveStatus string,
) ([]infodict.Format, map[string][]infodict.Subtitle, map[string][]infodict.Subtitle, error) {
	streamIDs := map[streamID]bool{}
	st := newFormatState(liveStatus)
	st.allFormats = hasFormatsArg(b, "duplicate")
	itags := st.itags

	// ---- pass 1: collect challenges -------------------------------------
	nChallenges := []string{}
	nSeen := map[string]bool{}
	sigLens := []int{}
	sigSeen := map[int]bool{}
	addN := func(v string) {
		if v != "" && !nSeen[v] {
			nSeen[v] = true
			nChallenges = append(nChallenges, v)
		}
	}
	for _, pr := range prs {
		sd, _ := pr["streamingData"].(map[string]any)
		if sd == nil {
			continue
		}
		for _, key := range []string{"formats", "adaptiveFormats"} {
			for _, fs := range asList(sd[key]) {
				f, _ := fs.(map[string]any)
				if f == nil {
					continue
				}
				fURL := utils.Str(f["url"])
				if fURL == "" {
					if sc := parseQueryString(utils.Str(f["signatureCipher"])); sc["s"] != nil {
						if s := sc["s"][0]; s != "" && !sigSeen[len(s)] {
							sigSeen[len(s)] = true
							sigLens = append(sigLens, len(s))
						}
					}
				}
				if fURL != "" {
					if q := parseQueryString(rawQuery(fURL)); q["n"] != nil {
						addN(q["n"][0])
					}
				}
			}
		}
		for _, mk := range []string{"hlsManifestUrl", "dashManifestUrl"} {
			if mu := utils.Str(sd[mk]); mu != "" {
				if m := reManifestN.FindStringSubmatch(pathOf(mu)); m != nil {
					addN(m[1])
				}
			}
		}
	}

	// ---- solve challenges ------------------------------------------------
	skipJS := hasSkip(b, "js")
	solvedN := map[string]string{}
	sigSpecs := map[int][]int{}
	if !skipJS && (len(nChallenges) > 0 || len(sigLens) > 0) {
		if err := b.loadPlayer(videoID); err == nil && b.playerJS != "" {
			if len(nChallenges) > 0 {
				res, err := jsc.SolveN(b.playerJS, nChallenges)
				if err != nil {
					b.ReportWarning("n challenge solving failed: %s (Some formats may be missing)", err)
				} else {
					solvedN = res
				}
			}
			if len(sigLens) > 0 {
				res, err := jsc.SolveSigSpec(b.playerJS, sigLens)
				if err != nil {
					b.ReportWarning("Signature solving failed: Some formats may be missing. (%s)", err)
				} else {
					sigSpecs = res
				}
			}
		}
	}

	// ---- pass 2: build formats ------------------------------------------
	var formats []infodict.Format
	seenSkippedURL := false
	for _, pr := range prs {
		sd, _ := pr["streamingData"].(map[string]any)
		if sd == nil {
			continue
		}
		clientName := utils.Str(sd["__yt_dlp_client"])
		client := InnertubeClients[clientName]
		if client == nil {
			continue
		}
		for _, key := range []string{"formats", "adaptiveFormats"} {
			for _, fs := range asList(sd[key]) {
				f, _ := fs.(map[string]any)
				if f == nil {
					continue
				}
				// live adaptive formats / OTF unsupported
				if f["targetDurationSec"] != nil {
					continue
				}
				if utils.Str(f["type"]) == "FORMAT_STREAM_TYPE_OTF" && f["drmFamilies"] == nil {
					continue
				}
				itag := utils.Str(f["itag"])
				track := ""
				if at, ok := f["audioTrack"].(map[string]any); ok {
					track = utils.Str(at["id"])
				}
				isDrc := f["isDrc"] != nil
				sid := streamID{itag: itag, track: track, isDrc: isDrc}
				if itag != "" && streamIDs[sid] {
					continue
				}

				potPolicy := client.gvsPolicy(ProtoHTTPS)
				requirePoToken := itag != "18" && gvsPotRequired(potPolicy, false, false)
				if requirePoToken && !hasFormatsArg(b, "missing_pot") {
					continue // no PO token provider in this milestone
				}

				fURL := utils.Str(f["url"])
				var encryptedSig, sp string
				if fURL == "" {
					sc := parseQueryString(utils.Str(f["signatureCipher"]))
					fURL = firstOf(sc["url"])
					encryptedSig = firstOf(sc["s"])
					sp = firstOf(sc["sp"])
					if fURL == "" || encryptedSig == "" || (b.playerURL == "" && !skipJS) {
						if !seenSkippedURL {
							seenSkippedURL = true
							b.WriteDebug("%s: some %s client https formats skipped: missing URL (SABR experiment?)", videoID, clientName)
						}
						continue
					}
				}

				fmtDict := b.processFormatStream(f, clientName, duration, requirePoToken, st)
				if fmtDict == nil {
					continue
				}

				if encryptedSig != "" {
					if skipJS {
						continue
					}
					spec, ok := sigSpecs[len(encryptedSig)]
					if !ok {
						continue
					}
					solved := jsc.ApplySig(encryptedSig, spec)
					if solved == "" {
						continue
					}
					if sp == "" {
						sp = "signature"
					}
					fURL += "&" + sp + "=" + url.QueryEscape(solved)
				}
				if q := parseQueryString(rawQuery(fURL)); q["n"] != nil {
					nVal := q["n"][0]
					if solved, ok := solvedN[nVal]; ok {
						fURL = updateURLQuery(fURL, map[string]string{"n": solved})
					} else if !skipJS {
						continue
					}
				}
				fmtDict.URL = fURL

				if itag != "" {
					if itags[itag] == nil {
						itags[itag] = map[string]bool{}
					}
					itags[itag]["https|"+fmtDict.Language] = true
					streamIDs[sid] = true
				}
				formats = append(formats, *fmtDict)
			}
		}
	}

	subs, autoCaps := b.extractSubtitles(prs)

	// ---- manifests: HLS (m3u8) -------------------------------------------
	for _, pr := range prs {
		sd, _ := pr["streamingData"].(map[string]any)
		if sd == nil {
			continue
		}
		clientName := utils.Str(sd["__yt_dlp_client"])
		b.addHLSManifestFormats(sd, clientName, videoID, liveStatus, solvedN, skipJS, st, &formats)
	}

	return formats, subs, autoCaps, nil
}

// addHLSManifestFormats downloads and parses a player response's HLS (m3u8)
// manifest, mirroring the hls block of _extract_formats_and_subtitles.
func (b *Base) addHLSManifestFormats(
	sd map[string]any, clientName, videoID, liveStatus string,
	solvedN map[string]string, skipJS bool, st *formatState, formats *[]infodict.Format,
) {
	hlsURL := utils.Str(sd["hlsManifestUrl"])
	if hlsURL == "" {
		return
	}
	if hasArgValue(b, "skip", "hls") {
		return
	}
	client := InnertubeClients[clientName]
	if client == nil {
		return
	}

	if nCh := manifestNChallenge(hlsURL); nCh != "" {
		res, ok := solvedN[nCh]
		if !ok {
			return
		}
		hlsURL = strings.ReplaceAll(hlsURL, "/n/"+nCh+"/", "/n/"+res+"/")
	}

	potPolicy := client.gvsPolicy(ProtoHLS)
	requirePoToken := gvsPotRequired(potPolicy, false, false)
	if requirePoToken && !hasFormatsArg(b, "missing_pot") {
		return // no PO token provider in this milestone
	}

	doc, err := b.DownloadWebpage(extractor.NewRequest(hlsURL, nil, nil), videoID,
		"Downloading m3u8 information", "Failed to download m3u8 information", false, "")
	if err != nil || doc == "" {
		return
	}
	if !strings.HasPrefix(doc, "#EXTM3U") {
		b.ReportWarning("Response data has no m3u header")
		return
	}

	fmts, _ := parseM3U8FormatsAndSubtitles(doc, hlsURL, "mp4", "m3u8_native")
	for i := range fmts {
		itag := ""
		if m := reM3U8Itag.FindStringSubmatch(fmts[i].URL); m != nil {
			itag = m[1]
		}
		if b.processManifestFormat(&fmts[i], "hls", clientName, itag, st) {
			*formats = append(*formats, fmts[i])
		}
	}
}

// processManifestFormat is the port of process_manifest_format.
func (b *Base) processManifestFormat(f *infodict.Format, proto, clientName, itag string, st *formatState) bool {
	key := proto + "|" + f.Language
	if st.itags[itag] == nil {
		st.itags[itag] = map[string]bool{}
	}
	itagSet := st.itags[itag]
	if !st.allFormats && itagSet[key] {
		return false
	}

	if f.SourcePreference == nil {
		sp := -1
		f.SourcePreference = &sp
	}
	// Deprioritize since these pre-merged m3u8 formats may have lower quality
	// audio streams.
	if (clientName == "web_safari" || clientName == "web_embedded") && proto == "hls" &&
		st.liveStatus != "is_live" && st.liveStatus != "post_live" {
		*f.SourcePreference--
	}

	itagSet[key] = true

	switch {
	case itag != "" && st.allFormats:
		f.FormatID = itag + "-" + proto
	case anyProtoDiff(itagSet, proto):
		f.FormatID = itag + "-" + proto
	case itag != "":
		f.FormatID = itag
	}

	switch {
	case f.Language != "" && f.Language == st.languageMap[10]:
		f.FormatNote = joinNonempty(" ", f.FormatNote, "(original)")
		lp := 10
		f.LanguagePreference = &lp
	case f.Language != "" && f.Language == st.languageMap[5]:
		f.FormatNote = joinNonempty(" ", f.FormatNote, "(default)")
		lp := 5
		f.LanguagePreference = &lp
	}

	if itag == "616" || itag == "235" {
		f.FormatNote = joinNonempty(" ", f.FormatNote, "Premium")
		*f.SourcePreference += 100
	}

	base := strings.SplitN(f.FormatID, "-", 2)[0]
	if q, ok := st.itagQualities[base]; ok {
		qv := qualityQ(q)
		f.Quality = &qv
	} else {
		qv := -1.0
		f.Quality = &qv
	}
	if *f.Quality == -1 && f.Height > 0 {
		if q, ok := st.nearestResQuality(f.Height); ok {
			qv := qualityQ(q)
			f.Quality = &qv
		}
	}
	if f.FPS <= 1 {
		f.FPS = 0
	}
	if proto == "hls" && truthyAny(f.HasDRM) {
		f.HasDRM = "maybe"
		*f.SourcePreference -= 5
	}
	return true
}

// qualityQ mirrors yt-dlp's qualities(...) lookup: the index in qualityOrder,
// or -1 when unknown.
func qualityQ(qid string) float64 {
	for i, name := range qualityOrder {
		if name == qid {
			return float64(i)
		}
	}
	return -1
}

// anyProtoDiff reports whether the itag set contains a protocol other than
// proto. Keys are encoded as "proto|language".
func anyProtoDiff(set map[string]bool, proto string) bool {
	for k := range set {
		p := k
		if i := strings.IndexByte(k, '|'); i >= 0 {
			p = k[:i]
		}
		if p != proto {
			return true
		}
	}
	return false
}

func truthyAny(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int:
		return t != 0
	case float64:
		return t != 0
	default:
		return true
	}
}

func manifestNChallenge(u string) string {
	if m := reManifestN.FindStringSubmatch(pathOf(u)); m != nil {
		return m[1]
	}
	return ""
}

func hasArgValue(b *Base, arg, item string) bool {
	for _, s := range b.GetExtractorArg(arg) {
		for _, part := range strings.Split(s, ",") {
			if strings.TrimSpace(part) == item {
				return true
			}
		}
	}
	return false
}

func hasFormatsArg(b *Base, item string) bool {
	for _, s := range b.GetExtractorArg("formats") {
		for _, part := range strings.Split(s, ",") {
			if strings.TrimSpace(part) == item {
				return true
			}
		}
	}
	return false
}

func asList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case nil:
		return nil
	default:
		return []any{t}
	}
}

func firstOf(ss []string) string {
	if len(ss) > 0 {
		return ss[0]
	}
	return ""
}

func rawQuery(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[i+1:]
	}
	return ""
}

func pathOf(u string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	return pu.Path
}

func updateURLQuery(u string, update map[string]string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	q := pu.Query()
	for k, v := range update {
		q.Set(k, v)
	}
	pu.RawQuery = q.Encode()
	return pu.String()
}

// processFormatStream is the port of process_format_stream.
func (b *Base) processFormatStream(fs map[string]any, clientName string, duration float64, missingPot bool, st *formatState) *infodict.Format {
	itag := utils.Str(fs["itag"])
	audioTrack, _ := fs["audioTrack"].(map[string]any)
	quality := utils.Str(fs["quality"])
	height := utils.SafeInt(fs["height"])
	if quality == "tiny" || quality == "" {
		if aq := strings.ToLower(utils.Str(fs["audioQuality"])); aq != "" {
			quality = aq
		}
	}
	if itag == "17" {
		quality = "tiny"
	}
	if quality != "" {
		if itag != "" {
			st.itagQualities[itag] = quality
		}
		if height > 0 {
			st.setResQuality(height, quality)
		}
	}

	hasDRM := fs["drmFamilies"] != nil
	if hasDRM {
		b.ReportWarning("Some %s client https formats have been skipped as they are DRM protected.", clientName)
		return nil
	}

	var tbr *float64
	if v := utils.FloatOrNone(fs["averageBitrate"]); v != nil {
		x := *v / 1000
		tbr = &x
	} else if v := utils.FloatOrNone(fs["bitrate"]); v != nil {
		x := *v / 1000
		tbr = &x
	}
	var formatDuration *float64
	if v := utils.FloatOrNone(fs["approxDurationMs"]); v != nil {
		x := *v / 1000
		formatDuration = &x
	}
	isDamaged := formatDuration != nil && duration > 0 && *formatDuration < duration/2

	if missingPot && !hasFormatsArg(b, "missing_pot") {
		return nil
	}

	displayName := ""
	if audioTrack != nil {
		displayName = utils.Str(audioTrack["displayName"])
		if audioTrack["audioIsDefault"] != nil {
			displayName = strings.TrimSpace(displayName + " (default)")
		}
	}
	name := utils.Str(fs["qualityLabel"])
	if name == "" {
		name = strings.ReplaceAll(quality, "audio_quality_", "")
	}
	fps := utils.SafeInt(fs["fps"])
	if fps <= 1 {
		fps = 0
	}
	projection := strings.ReplaceAll(utils.Str(fs["projectionType"]), "RECTANGULAR", "")
	projection = strings.ToLower(projection)
	spatial := strings.ReplaceAll(utils.Str(fs["spatialAudioType"]), "SPATIAL_AUDIO_TYPE_", "")
	spatial = strings.ToLower(spatial)

	noteParts := []string{}
	if displayName != "" {
		noteParts = append(noteParts, displayName)
	}
	if name != "" {
		noteParts = append(noteParts, name)
	}
	if fs["isDrc"] != nil {
		noteParts = append(noteParts, "DRC")
	}
	if projection != "" {
		noteParts = append(noteParts, projection)
	}
	if spatial != "" {
		noteParts = append(noteParts, spatial)
	}
	if isDamaged {
		noteParts = append(noteParts, "DAMAGED")
	}
	if missingPot {
		noteParts = append(noteParts, "MISSING POT")
	}

	sourcePref := int64(-1)
	if itag == "22" {
		sourcePref = -5
	}
	if strings.Contains(name, "Premium") {
		sourcePref += 100
	}

	var pref *int
	if isDamaged {
		pref = ip(-10)
	} else if itag == "17" {
		pref = ip(-2)
	}

	formatID := itag
	if fs["isDrc"] != nil {
		formatID += "-drc"
	}

	language := ""
	var langPref int64 = -1
	if audioTrack != nil {
		language = strings.SplitN(utils.Str(audioTrack["id"]), ".", 2)[0]
		display := strings.ToLower(utils.Str(audioTrack["displayName"]))
		switch {
		case strings.Contains(display, "descriptive"):
			language += "-desc"
			langPref = -10
		case strings.Contains(display, "original"):
			langPref = 10
			if language != "" && st.languageMap[10] == "" {
				st.languageMap[10] = language
			}
		case audioTrack["audioIsDefault"] != nil:
			langPref = 5
			if language != "" && st.languageMap[5] == "" {
				st.languageMap[5] = language
			}
		}
	}

	out := &infodict.Format{
		FormatID:           formatID,
		FormatNote:         strings.Join(noteParts, ", "),
		Filesize:           i64d(utils.IntOrNone(fs["contentLength"])),
		FPS:                float64(fps),
		Height:             height,
		Width:              utils.SafeInt(fs["width"]),
		TBR:                f64d(tbr),
		AudioChannels:      utils.SafeInt(fs["audioChannels"]),
		ASR:                utils.SafeInt(fs["audioSampleRate"]),
		FilesizeApprox:     i64d(utils.FilesizeFromTbr(tbr, formatDuration)),
		Language:           language,
		SourcePreference:   ip(sourcePref),
		Preference:         pref,
		LanguagePreference: ip(langPref),
		HasDRM:             hasDRM,
		DownloaderOptions:  map[string]any{"http_chunk_size": 10 << 20},
	}
	out.Quality = qualityIndex(quality)
	// yt-dlp deprioritizes DRC (dynamic range compressed) formats via a
	// half-point quality penalty (see youtube/_video.py).
	if fs["isDrc"] != nil && out.Quality != nil {
		v := *out.Quality - 0.5
		out.Quality = &v
	}

	if m := reMimeType.FindStringSubmatch(utils.Str(fs["mimeType"])); m != nil {
		out.Ext = utils.Mimetype2ext(strings.Split(m[1], ";")[0])
		vc, ac, hdr := utils.ParseCodecs(m[2])
		out.VCodec, out.ACodec = vc, ac
		if hdr != "" {
			out.DynamicRange = hdr
		}
	}
	if out.ACodec == "none" || out.VCodec == "none" {
		out.Container = out.Ext + "_dash"
	}
	return out
}

func ip(v int64) *int { i := int(v); return &i }
func i64d(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
func f64d(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// qualityOrder mirrors yt-dlp's YouTube `qualities([...])` list.
var qualityOrder = []string{
	"tiny",
	"audio_quality_ultralow", "audio_quality_low", "audio_quality_medium", "audio_quality_high",
	"small", "medium", "large", "hd720", "hd1080", "hd1440", "hd2160", "hd2880", "highres",
}

func qualityIndex(q string) *float64 {
	if q == "" {
		return nil
	}
	for i, name := range qualityOrder {
		if name == q {
			v := float64(i)
			return &v
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Subtitles / automatic captions
// ---------------------------------------------------------------------------

func (b *Base) extractSubtitles(prs []map[string]any) (
	subs, autoCaps map[string][]infodict.Subtitle) {
	subs = map[string][]infodict.Subtitle{}
	autoCaps = map[string][]infodict.Subtitle{}
	seen := map[string]bool{}
	for _, pr := range prs {
		if pr["streamingData"] == nil {
			continue
		}
		capList, _ := utils.Traverse(pr, "captions.playerCaptionsTracklistRenderer").(map[string]any)
		if capList == nil {
			continue
		}
		translationLangs := map[string]string{}
		for _, tl := range asList(capList["translationLanguages"]) {
			tlm, _ := tl.(map[string]any)
			if tlm == nil {
				continue
			}
			code := utils.Str(tlm["languageCode"])
			translationLangs[code] = textOf(tlm["languageName"])
		}
		for _, tr := range asList(capList["captionTracks"]) {
			track, _ := tr.(map[string]any)
			if track == nil {
				continue
			}
			base := utils.Str(track["baseUrl"])
			if base == "" {
				continue
			}
			lang := utils.Str(track["languageCode"])
			name := textOf(track["name"])
			isASR := utils.Str(track["kind"]) == "asr"
			target := subs
			kind := "m"
			if isASR {
				target, kind = autoCaps, "a"
			}
			addCaption(target, seen, kind, lang, base, "", name)
			if track["isTranslatable"] != nil {
				var tlangs []string
				for k := range translationLangs {
					tlangs = append(tlangs, k)
				}
				sort.Strings(tlangs)
				for _, tl := range tlangs {
					addCaption(target, seen, kind, tl, base, tl, translationLangs[tl])
				}
			}
		}
	}
	return subs, autoCaps
}

// subtitleFormats lists the caption representations YouTube serves from a
// track baseUrl, ordered from lowest to highest preference (like yt-dlp). An
// empty fmt keeps the default srv1 response.
var subtitleFormats = []struct{ fmt, ext string }{
	{"json3", "json3"},
	{"", "srv1"},
	{"srv2", "srv2"},
	{"srv3", "srv3"},
	{"ttml", "ttml"},
	{"vtt", "vtt"},
}

// addCaption appends the full set of caption representations for a track to
// target[lang], skipping duplicates (the same track is present in every player
// response). tlang selects a translated variant when non-empty.
func addCaption(target map[string][]infodict.Subtitle, seen map[string]bool, kind, lang, base, tlang, name string) {
	for _, f := range subtitleFormats {
		sub := captionSubtitle(base, f.fmt, tlang, name)
		key := kind + "\x00" + lang + "\x00" + sub.Ext
		if seen[key] {
			continue
		}
		seen[key] = true
		target[lang] = append(target[lang], sub)
	}
}

// captionSubtitle builds one subtitle representation URL for a caption track.
func captionSubtitle(base, fmtParam, tlang, name string) infodict.Subtitle {
	u := base
	if fmtParam != "" {
		u += "&fmt=" + fmtParam
	}
	if tlang != "" {
		u += "&tlang=" + tlang
	}
	ext := fmtParam
	if ext == "" {
		ext = "srv1"
	}
	return infodict.Subtitle{Ext: ext, URL: u, Name: name}
}

// textOf extracts plain text from a renderer (simpleText or runs).
func textOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s := utils.Str(t["simpleText"]); s != "" {
			return s
		}
		var sb strings.Builder
		for _, r := range asList(t["runs"]) {
			rm, _ := r.(map[string]any)
			if rm != nil {
				sb.WriteString(utils.Str(rm["text"]))
			}
		}
		return sb.String()
	}
	return ""
}
