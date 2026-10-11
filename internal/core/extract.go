package core

import (
	"fmt"
	mathrand "math/rand"
	"strings"
	"time"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// DownloadFunc downloads the selected formats of an info dict.
type DownloadFunc func(info *infodict.InfoDict, formats []infodict.Format) error

// SetDownloader wires the download stage.
func (y *YoutubeDL) SetDownloader(fn DownloadFunc) { y.downloader = fn }

// ExtractInfo resolves a URL to an info dict, optionally downloading it.
func (y *YoutubeDL) ExtractInfo(url string, download bool) (*infodict.InfoDict, error) {
	ie := y.getInfoExtractor(url, "")
	if ie == nil {
		return nil, &extractor.UnsupportedURLError{URL: url}
	}
	ie.SetDownloader(y)
	if ps, ok := ie.(interface{ SetPlaylistMaxEntries(int) }); ok {
		ps.SetPlaylistMaxEntries(y.playlistMaxEntries())
	}
	result, err := ie.Extract(url)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("extractor returned no result for %s", url)
	}
	return y.processIeResult(result, download, map[string]string{"original_url": url}, 0)
}

// playlistMaxEntries computes how many playlist entries the extractor must
// collect (0 = everything) so that pagination stops early for windows that
// only cover the beginning of a playlist.
func (y *YoutubeDL) playlistMaxEntries() int {
	it := y.opts.PlaylistItems
	if it != "" {
		ranges, err := parsePlaylistItems(it)
		if err != nil {
			return 0
		}
		max := 0
		for _, r := range ranges {
			if r.endUnbounded || r.step < 0 || r.start < 0 || r.end < 0 || r.start == 0 {
				return 0
			}
			if r.end > max {
				max = r.end
			}
			if r.start > max {
				max = r.start
			}
		}
		return max
	}
	if y.opts.PlaylistReverse || y.opts.PlaylistRandom {
		return 0
	}
	if y.opts.PlaylistEnd > 0 {
		return y.opts.PlaylistEnd
	}
	return 0
}

func (y *YoutubeDL) processIeResult(result *infodict.InfoDict, download bool, extra map[string]string, depth int) (*infodict.InfoDict, error) {
	if depth > 20 {
		return nil, fmt.Errorf("maximum recursion depth reached processing %s", result.ID)
	}
	rtype := result.Type
	if rtype == "" {
		rtype = "video"
	}

	switch rtype {
	case "video":
		return y.processVideoResult(result, download)
	case "url":
		return y.ExtractInfo(result.URL, download)
	case "url_transparent":
		inner, err := y.extractInfoNoProcess(result, extra)
		if err != nil {
			return nil, err
		}
		if inner == nil {
			return nil, nil
		}
		merged := inner.Clone()
		mergeTransparent(merged, result)
		return y.processIeResult(merged, download, extra, depth+1)
	case "playlist", "multi_video":
		return y.processPlaylist(result, download, extra, depth)
	default:
		return nil, fmt.Errorf("invalid result type: %s", rtype)
	}
}

// extractInfoNoProcess extracts a url result without finalizing it.
func (y *YoutubeDL) extractInfoNoProcess(result *infodict.InfoDict, extra map[string]string) (*infodict.InfoDict, error) {
	ieKey := result.IEKey
	if ieKey == "" {
		ieKey = result.ExtractorKey
	}
	ie := y.getInfoExtractor(result.URL, ieKey)
	if ie == nil {
		ie = y.getInfoExtractor(result.URL, "")
	}
	if ie == nil {
		return nil, &extractor.UnsupportedURLError{URL: result.URL}
	}
	ie.SetDownloader(y)
	if ps, ok := ie.(interface{ SetPlaylistMaxEntries(int) }); ok {
		ps.SetPlaylistMaxEntries(y.playlistMaxEntries())
	}
	return ie.Extract(result.URL)
}

func mergeTransparent(dst, src *infodict.InfoDict) {
	if src.Title != "" && dst.Title == "" {
		dst.Title = src.Title
	}
	if src.ID != "" && dst.ID == "" {
		dst.ID = src.ID
	}
	if src.Type == "url" {
		dst.Type = "url_transparent"
	}
}

func (y *YoutubeDL) processPlaylist(result *infodict.InfoDict, download bool, extra map[string]string, depth int) (*infodict.InfoDict, error) {
	if result.WebpageURL != "" {
		if y.playlistURL[result.WebpageURL] {
			y.ToScreen("[download] Skipping already downloaded playlist: %s", orDefault(result.Title, result.ID))
			return result, nil
		}
		y.playlistURL[result.WebpageURL] = true
	}
	y.playlistLvl++
	defer func() {
		y.playlistLvl--
		if y.playlistLvl == 0 {
			y.playlistURL = map[string]bool{}
		}
	}()

	y.fillCommonFields(result, false)
	y.sortThumbnails(result)

	if !y.opts.NoPlaylist {
		selected, err := selectPlaylistEntries(result.Entries, y.opts.PlaylistStart, y.opts.PlaylistEnd, y.opts.PlaylistItems)
		if err != nil {
			return nil, err
		}
		if y.opts.PlaylistReverse {
			reverseItems(selected)
		}
		if y.opts.PlaylistRandom {
			shuffleItems(selected)
		}
		count := len(selected)
		y.ToScreen("[download] Downloading playlist: %s", orDefault(result.Title, result.ID))
		y.ToScreen("[download] Playlist %s: Downloading %d videos", orDefault(result.Title, result.ID), count)
		for _, it := range selected {
			entry := it.entry.Clone()
			entry.Playlist = orDefault(result.Title, result.ID)
			idx := it.idx
			entry.PlaylistIndex = &idx
			if result.ID != "" {
				entry.PlaylistID = result.ID
			}
			wasArchived := y.opts.DownloadArchive != "" && y.inArchive(entry)
			errorsBefore := y.Errors
			if _, err := y.processIeResult(entry, download, extra, depth+1); err != nil {
				y.ReportError("%v", err)
			}
			if wasArchived && y.opts.BreakOnExisting {
				y.ToScreen("[download] Stopping because of --break-on-existing")
				break
			}
			if y.opts.BreakPerURL && y.Errors > errorsBefore {
				break
			}
		}
	}
	return result, nil
}

func reverseItems(items []playlistItem) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func shuffleItems(items []playlistItem) {
	mathrand.New(mathrand.NewSource(time.Now().UnixNano())).Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}

type playlistItem struct {
	idx   int
	entry *infodict.InfoDict
}

func (y *YoutubeDL) processVideoResult(info *infodict.InfoDict, download bool) (*infodict.InfoDict, error) {
	if info.ID == "" {
		return nil, fmt.Errorf("missing \"id\" field in extractor result")
	}
	if info.DisplayID == "" {
		info.DisplayID = info.ID
	}
	y.sortThumbnails(info)
	y.fillCommonFields(info, true)

	formats := make([]infodict.Format, 0, len(info.Formats))
	for _, f := range info.Formats {
		if f.URL == "" && len(f.Fragments) == 0 {
			y.ReportWarning("\"url\" field is missing or empty - skipping format")
			continue
		}
		formats = append(formats, f)
	}
	if len(formats) == 0 {
		return nil, &extractor.NoFormatsError{Msg: "No video formats found", Expected: true, VideoID: info.ID}
	}

	// sort
	sortFormats(formats, &optionsSortParams{
		preferFree: y.opts.PreferFreeFormats,
		sortUser:   y.opts.FormatSort,
		forceUser:  y.opts.FormatSortForce,
	})

	spec := y.opts.Format
	if spec == "" {
		spec = y.defaultFormatSpec(info)
	}
	selected, err := y.selectFormats(formats, spec)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, &extractor.NoFormatsError{
			Msg:      fmt.Sprintf("Requested format is not available: %s", spec),
			Expected: true, VideoID: info.ID,
		}
	}
	primary := selected[0]
	info.SelectedFormats = selected
	if len(primary.RequestedFormats) > 0 {
		info.RequestedFormats = primary.RequestedFormats
	}
	info.URL = primary.URL
	info.Ext = primary.Ext
	info.Format = primary.Format
	info.FormatID = primary.FormatID
	info.VCodec = primary.VCodec
	info.ACodec = primary.ACodec
	info.Width = primary.Width
	info.Height = primary.Height
	info.Resolution = formatResolution(primary)
	if primary.FPS != 0 {
		info.FPS = primary.FPS
	}
	info.TBR = primary.TBR
	info.Protocol = primary.Protocol
	info.Formats = formats

	if download {
		if err := y.processInfo(info); err != nil {
			return info, err
		}
	}
	return info, nil
}

// processInfo builds the output filename and triggers the download.
func (y *YoutubeDL) processInfo(info *infodict.InfoDict) error {
	if y.opts.SkipDownload {
		return nil
	}
	if y.opts.DownloadArchive != "" && !y.forceDownload(info) && y.inArchive(info) {
		y.ToScreen("[download] %s has already been recorded in archive", archiveKey(info))
		return nil
	}
	path, err := y.buildOutputPath(info)
	if err != nil {
		return err
	}
	info.Filepath = path
	if y.downloader == nil {
		y.ToScreen("[download] Destination: %s", path)
		return nil
	}
	formats := info.RequestedFormats
	if len(formats) == 0 {
		formats = info.SelectedFormats
	}
	if err := y.downloader(info, formats); err != nil {
		return err
	}
	if y.opts.DownloadArchive != "" {
		if err := y.addArchive(info); err != nil {
			y.ReportWarning("Unable to add to archive: %v", err)
		}
	}
	if y.subtitlesRequested() {
		y.writeSubtitles(info)
	}
	if y.thumbnailsRequested() {
		y.writeThumbnails(info)
	}
	if y.opts.WriteInfoJSON {
		if err := y.writeInfoJSON(info, info.Filepath); err != nil {
			y.ReportWarning("Unable to write info.json: %v", err)
		}
	}
	return nil
}

// forceDownload reports whether --force-overwrites forces scripts to go ahead
// even if the entry is already recorded in the archive.
func (y *YoutubeDL) forceDownload(info *infodict.InfoDict) bool {
	return y.opts.Overwrites != nil && *y.opts.Overwrites
}

func (y *YoutubeDL) defaultFormatSpec(info *infodict.InfoDict) string {
	if y.opts.Outtmpl["default"] == "-" {
		return "best/bestvideo+bestaudio"
	}
	return "bestvideo*+bestaudio/best"
}

// ---------------------------------------------------------------------------
// Common fields
// ---------------------------------------------------------------------------

func (y *YoutubeDL) fillCommonFields(info *infodict.InfoDict, final bool) {
	if final {
		if info.Title == "" {
			info.Title = fmt.Sprintf("%s video #%s", strings.ReplaceAll(info.Extractor, ":", "-"), info.ID)
		}
	}
	if info.Duration > 0 && info.DurationString == "" {
		info.DurationString = formatSeconds(info.Duration)
	}
	if info.UploadDate == "" && info.Timestamp != nil {
		info.UploadDate = time.Unix(*info.Timestamp, 0).UTC().Format("20060102")
	}
	if info.ReleaseDate == "" && info.ReleaseTimestamp != nil {
		info.ReleaseDate = time.Unix(*info.ReleaseTimestamp, 0).UTC().Format("20060102")
	}
	if info.ReleaseYear == nil && len(info.ReleaseDate) >= 4 {
		if n, err := parseInt(info.ReleaseDate[:4]); err == nil {
			info.ReleaseYear = &n
		}
	}
	if info.LiveStatus == "" {
		if info.IsLive != nil && *info.IsLive {
			info.LiveStatus = "is_live"
		} else if info.WasLive != nil && *info.WasLive {
			info.LiveStatus = "was_live"
		}
	}
	for field, numField := range map[string]**int64{
		"chapter": &info.ChapterNumber,
		"season":  &info.SeasonNumber,
		"episode": &info.EpisodeNumber,
	} {
		_ = field
		_ = numField
	}
}

func formatSeconds(d float64) string {
	total := int(d + 0.5)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func parseInt(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func formatResolution(f infodict.Format) string {
	if f.Resolution != "" {
		return f.Resolution
	}
	if f.Height > 0 {
		if f.Width > 0 {
			return fmt.Sprintf("%dx%d", f.Width, f.Height)
		}
		return fmt.Sprintf("%dp", f.Height)
	}
	if f.Width > 0 {
		return fmt.Sprintf("%dx?", f.Width)
	}
	return "audio only"
}

func sortFormats(formats []infodict.Format, p *optionsSortParams) {
	s := newFormatSorter(p)
	// stable insertion sort by preference key
	keys := make([][]float64, len(formats))
	for i := range formats {
		keys[i] = s.calculatePreference(&formats[i])
	}
	// insertion sort (stable) ascending
	for i := 1; i < len(formats); i++ {
		for j := i; j > 0 && compareKeys(keys[j-1], keys[j]) > 0; j-- {
			formats[j-1], formats[j] = formats[j], formats[j-1]
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
}

func compareKeys(a, b []float64) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

var _ = utils.Str
