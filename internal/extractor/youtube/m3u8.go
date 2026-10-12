package youtube

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// This file ports the subset of yt-dlp's HLS (m3u8) handling needed by the
// YouTube extractor: `parse_m3u8_attributes` and the master-playlist branch of
// `_parse_m3u8_formats_and_subtitles` (see extractor/common.py). Media
// playlists are returned as a single format; live/split-discontinuity handling
// is out of scope.

var (
	reM3U8Attr          = regexp.MustCompile(`([A-Z0-9-]+)=("[^"]+"|[^",]+)(?:,|$)`)
	reM3U8Resolution    = regexp.MustCompile(`(\d+)[xX](\d+)`)
	reM3U8UnifiedStream = regexp.MustCompile(`audio.*?(?:%3D|=)(\d+)(?:-video.*?(?:%3D|=)(\d+))?`)
	reM3U8DRM           = regexp.MustCompile(`#EXT-X-KEY:[^\n]*METHOD=(SAMPLE-AES|SAMPLE-AES-CTR)`)
	reM3U8Itag          = regexp.MustCompile(`/itag/(\d+)`)
)

// parseM3U8Attributes is the port of parse_m3u8_attributes.
func parseM3U8Attributes(line string) map[string]string {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		line = line[i+1:]
	}
	attrs := map[string]string{}
	for _, m := range reM3U8Attr.FindAllStringSubmatch(line, -1) {
		v := m[2]
		if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
			v = v[1 : len(v)-1]
		}
		attrs[m[1]] = v
	}
	return attrs
}

// hlsHasDRM is the port of HlsFD._has_drm.
func hlsHasDRM(doc string) bool {
	return reM3U8DRM.MatchString(doc)
}

// determineExt mirrors yt-dlp's determine_ext for URL-like strings.
func determineExt(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.LastIndexByte(u, '.'); i >= 0 {
		return u[i+1:]
	}
	return ""
}

func isAbsoluteURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func formatURL(base, ref string) string {
	if isAbsoluteURL(ref) {
		return ref
	}
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

// joinNonempty mirrors yt-dlp's join_nonempty.
func joinNonempty(delim string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, delim)
}

// parseM3U8FormatsAndSubtitles is the port of the master-playlist branch of
// _parse_m3u8_formats_and_subtitles. m3u8ID, preference and quality are always
// empty/None for the YouTube call site.
func parseM3U8FormatsAndSubtitles(doc, m3u8URL, ext, entryProtocol string) ([]infodict.Format, map[string][]infodict.Subtitle) {
	formats := []infodict.Format{}
	subtitles := map[string][]infodict.Subtitle{}
	hasDRM := hlsHasDRM(doc)

	if strings.Contains(doc, "#EXT-X-TARGETDURATION") { // media playlist, return as is
		return []infodict.Format{{
			URL:      m3u8URL,
			Ext:      ext,
			Protocol: entryProtocol,
			HasDRM:   hasDRM,
		}}, subtitles
	}

	groups := map[string][]map[string]string{}
	var lastStreamInf map[string]string

	// audioGroupIDs tracks the (internal) `_audio_group_id` of each format.
	var audioGroupIDs []string

	extractMedia := func(line string) {
		media := parseM3U8Attributes(line)
		mediaType, groupID, name := media["TYPE"], media["GROUP-ID"], media["NAME"]
		if mediaType == "" || groupID == "" || name == "" {
			return
		}
		groups[groupID] = append(groups[groupID], media)
		if mediaType == "SUBTITLES" {
			if media["URI"] == "" {
				return
			}
			u := formatURL(m3u8URL, media["URI"])
			sext := determineExt(u)
			proto := ""
			if sext == "m3u8" {
				sext = "vtt"
				proto = "m3u8_native"
			}
			lang := media["LANGUAGE"]
			if lang == "" {
				lang = "und"
			}
			subtitles[lang] = append(subtitles[lang], infodict.Subtitle{Ext: sext, URL: u, Protocol: proto})
		}
		if mediaType != "VIDEO" && mediaType != "AUDIO" {
			return
		}
		mediaURL := media["URI"]
		if mediaURL == "" {
			return
		}
		manifestURL := formatURL(m3u8URL, mediaURL)
		isAudio := mediaType == "AUDIO"
		isAlternate := media["DEFAULT"] == "NO" || media["AUTOSELECT"] == "NO"
		f := infodict.Format{
			FormatID:    joinNonempty("-", "", groupID, name, ""),
			FormatNote:  name,
			URL:         manifestURL,
			ManifestURL: m3u8URL,
			Language:    media["LANGUAGE"],
			Ext:         ext,
			Protocol:    entryProtocol,
			HasDRM:      hasDRM,
		}
		if isAudio {
			f.VCodec = "none"
		}
		if isAudio && isAlternate {
			sp := -2
			f.SourcePreference = &sp
		}
		formats = append(formats, f)
		if isAudio && !isAlternate {
			audioGroupIDs = append(audioGroupIDs, groupID)
		} else {
			audioGroupIDs = append(audioGroupIDs, "")
		}
	}

	buildStreamName := func() string {
		if n := lastStreamInf["NAME"]; n != "" {
			return n
		}
		groupID := lastStreamInf["VIDEO"]
		if groupID == "" {
			return ""
		}
		group := groups[groupID]
		if len(group) == 0 {
			return groupID
		}
		if n := group[0]["NAME"]; n != "" {
			return n
		}
		return groupID
	}

	// parse EXT-X-MEDIA tags before EXT-X-STREAM-INF
	lines := strings.Split(doc, "\n")
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "#EXT-X-MEDIA:") {
			extractMedia(line)
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			lastStreamInf = parseM3U8Attributes(line)
		case strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "":
			continue
		default:
			tbr := firstFloatAttr(lastStreamInf, "AVERAGE-BANDWIDTH", "BANDWIDTH")
			if tbr != nil {
				v := *tbr / 1000
				tbr = &v
			}
			manifestURL := formatURL(m3u8URL, strings.TrimSpace(line))

			idPart := buildStreamName()
			if idPart == "" {
				if tbr != nil {
					idPart = strconv.Itoa(int(*tbr))
				} else {
					idPart = strconv.Itoa(len(formats))
				}
			}

			f := infodict.Format{
				FormatID:    joinNonempty("-", "", idPart, ""),
				URL:         manifestURL,
				ManifestURL: m3u8URL,
				Ext:         ext,
				Protocol:    entryProtocol,
				HasDRM:      hasDRM,
			}
			if tbr != nil {
				f.TBR = *tbr
			}
			if fr := lastStreamInf["FRAME-RATE"]; fr != "" {
				if v, err := strconv.ParseFloat(fr, 64); err == nil {
					f.FPS = v
				}
			}
			// YouTube-specific: audio track language
			if acID := lastStreamInf["YT-EXT-AUDIO-CONTENT-ID"]; acID != "" {
				f.Language = strings.SplitN(acID, ".", 2)[0]
			}
			if res := lastStreamInf["RESOLUTION"]; res != "" {
				if m := reM3U8Resolution.FindStringSubmatch(res); m != nil {
					f.Width, _ = strconv.Atoi(m[1])
					f.Height, _ = strconv.Atoi(m[2])
				}
			}
			// Unified Streaming Platform
			if m := reM3U8UnifiedStream.FindStringSubmatch(f.URL); m != nil {
				if a, err := strconv.ParseFloat(m[1], 64); err == nil {
					abr := a / 1000
					f.ABR = &abr
				}
				if m[2] != "" {
					if v, err := strconv.ParseFloat(m[2], 64); err == nil {
						vbr := v / 1000
						f.VBR = &vbr
					}
				}
			}
			f.VCodec, f.ACodec, _ = utils.ParseCodecs(lastStreamInf["CODECS"])

			var groupIDForAudio string
			audioGroupID := lastStreamInf["AUDIO"]
			if audioGroupID != "" && lastStreamInf["CODECS"] != "" && f.VCodec != "none" {
				groupIDForAudio = audioGroupID
				if ag := groups[audioGroupID]; len(ag) > 0 && ag[0]["URI"] != "" {
					f.ACodec = "none"
				}
			}
			if f.Ext == "" {
				if f.VCodec == "none" {
					f.Ext = "m4a"
				} else {
					f.Ext = "mp4"
				}
			}
			formats = append(formats, f)
			audioGroupIDs = append(audioGroupIDs, groupIDForAudio)
			lastStreamInf = nil
		}
	}

	// Some audio-only formats only have a GROUP-ID; set source_preference based
	// on the quality of the video formats they are grouped with.
	type groupRank struct {
		gid   string
		tbr   float64
		width int
	}
	var ranks []groupRank
	for i := range formats {
		if formats[i].VCodec != "none" && audioGroupIDs[i] != "" {
			ranks = append(ranks, groupRank{audioGroupIDs[i], formats[i].TBR, formats[i].Width})
		}
	}
	sort.SliceStable(ranks, func(a, b int) bool {
		if ranks[a].tbr != ranks[b].tbr {
			return ranks[a].tbr < ranks[b].tbr
		}
		return ranks[a].width < ranks[b].width
	})
	var ordered []string
	seenGID := map[string]bool{}
	for _, r := range ranks {
		if !seenGID[r.gid] {
			seenGID[r.gid] = true
			ordered = append(ordered, r.gid)
		}
	}
	var qualityMap map[string]string
	if len(ordered) > 1 {
		qualityMap = map[string]string{ordered[0]: "low", ordered[len(ordered)-1]: "high"}
	}
	prefIndex := func(gid string) int {
		for i, g := range ordered {
			if g == gid {
				return i
			}
		}
		return -1
	}
	for i := range formats {
		gid := audioGroupIDs[i]
		if qualityMap == nil || gid == "" || formats[i].VCodec != "none" {
			continue
		}
		sp := prefIndex(gid)
		formats[i].SourcePreference = &sp
		formats[i].FormatNote = joinNonempty(", ", formats[i].FormatNote, qualityMap[gid])
	}

	return formats, subtitles
}

// firstFloatAttr returns the first present numeric attribute.
func firstFloatAttr(attrs map[string]string, keys ...string) *float64 {
	for _, k := range keys {
		if v := attrs[k]; v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
			}
		}
	}
	return nil
}
