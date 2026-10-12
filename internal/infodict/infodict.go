// Package infodict defines the media information dictionary, the lingua franca
// exchanged between extractors, the core pipeline and downloaders.
// It mirrors yt-dlp's InfoDict documented in extractor/common.py.
package infodict

// InfoDict is a single extracted result: a video, a playlist or a redirect.
type InfoDict struct {
	// Type discriminates the result: "video" (zero value), "playlist",
	// "multi_video", "url", "url_transparent".
	Type string `json:"_type,omitempty"`

	// Identity
	ID                 string `json:"id,omitempty"`
	Title              string `json:"title,omitempty"`
	DisplayID          string `json:"display_id,omitempty"`
	AltTitle           string `json:"alt_title,omitempty"`
	Ext                string `json:"ext,omitempty"`
	Format             string `json:"format,omitempty"`
	URL                string `json:"url,omitempty"`
	IEKey              string `json:"ie_key,omitempty"`
	WebpageURL         string `json:"webpage_url,omitempty"`
	OriginalURL        string `json:"original_url,omitempty"`
	WebpageURLBaseName string `json:"webpage_url_basename,omitempty"`
	WebpageURLDomain   string `json:"webpage_url_domain,omitempty"`
	Extractor          string `json:"extractor,omitempty"`
	ExtractorKey       string `json:"extractor_key,omitempty"`
	Direct             bool   `json:"direct,omitempty"`
	PlayerURL          string `json:"player_url,omitempty"`

	// People / channel
	Uploader             string   `json:"uploader,omitempty"`
	UploaderID           string   `json:"uploader_id,omitempty"`
	UploaderURL          string   `json:"uploader_url,omitempty"`
	Channel              string   `json:"channel,omitempty"`
	ChannelID            string   `json:"channel_id,omitempty"`
	ChannelURL           string   `json:"channel_url,omitempty"`
	ChannelFollowerCount *int64   `json:"channel_follower_count,omitempty"`
	ChannelIsVerified    *bool    `json:"channel_is_verified,omitempty"`
	Location             string   `json:"location,omitempty"`
	License              string   `json:"license,omitempty"`
	Creators             []string `json:"creators,omitempty"`

	// Dates (unix timestamps in seconds)
	Timestamp         *int64 `json:"timestamp,omitempty"`
	UploadDate        string `json:"upload_date,omitempty"`
	ReleaseTimestamp  *int64 `json:"release_timestamp,omitempty"`
	ReleaseDate       string `json:"release_date,omitempty"`
	ReleaseYear       *int64 `json:"release_year,omitempty"`
	ModifiedTimestamp *int64 `json:"modified_timestamp,omitempty"`
	ModifiedDate      string `json:"modified_date,omitempty"`

	// Metrics
	Duration            float64  `json:"duration,omitempty"`
	DurationString      string   `json:"duration_string,omitempty"`
	ViewCount           *int64   `json:"view_count,omitempty"`
	ConcurrentViewCount *int64   `json:"concurrent_view_count,omitempty"`
	LikeCount           *int64   `json:"like_count,omitempty"`
	DislikeCount        *int64   `json:"dislike_count,omitempty"`
	CommentCount        *int64   `json:"comment_count,omitempty"`
	RepostCount         *int64   `json:"repost_count,omitempty"`
	SaveCount           *int64   `json:"save_count,omitempty"`
	AverageRating       *float64 `json:"average_rating,omitempty"`

	// Classification / state
	Description     string   `json:"description,omitempty"`
	Categories      []string `json:"categories,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	AgeLimit        *int     `json:"age_limit,omitempty"`
	IsLive          *bool    `json:"is_live,omitempty"`
	WasLive         *bool    `json:"was_live,omitempty"`
	LiveStatus      string   `json:"live_status,omitempty"`
	PlayableInEmbed *bool    `json:"playable_in_embed,omitempty"`
	Availability    string   `json:"availability,omitempty"`
	MediaType       string   `json:"media_type,omitempty"`

	// Playback
	StartTime          *float64              `json:"start_time,omitempty"`
	EndTime            *float64              `json:"end_time,omitempty"`
	Thumbnail          string                `json:"thumbnail,omitempty"`
	Thumbnails         []Thumbnail           `json:"thumbnails,omitempty"`
	Chapters           []Chapter             `json:"chapters,omitempty"`
	Heatmap            []HeatmapSegment      `json:"heatmap,omitempty"`
	Subtitles          map[string][]Subtitle `json:"subtitles,omitempty"`
	AutomaticCaptions  map[string][]Subtitle `json:"automatic_captions,omitempty"`
	RequestedSubtitles map[string]Subtitle   `json:"requested_subtitles,omitempty"`
	Formats            []Format              `json:"formats,omitempty"`
	RequestedFormats   []Format              `json:"requested_formats,omitempty"`
	SelectedFormats    []Format              `json:"-"`
	FilesToMove        map[string]string     `json:"-"`

	// Series / episode
	Series        string `json:"series,omitempty"`
	SeriesID      string `json:"series_id,omitempty"`
	Season        string `json:"season,omitempty"`
	SeasonNumber  *int64 `json:"season_number,omitempty"`
	Episode       string `json:"episode,omitempty"`
	EpisodeNumber *int64 `json:"episode_number,omitempty"`
	Chapter       string `json:"chapter,omitempty"`
	ChapterNumber *int64 `json:"chapter_number,omitempty"`

	// Track / music
	Track   string   `json:"track,omitempty"`
	Album   string   `json:"album,omitempty"`
	Artists []string `json:"artists,omitempty"`
	Genres  []string `json:"genres,omitempty"`

	// Playlists
	PlaylistCount *int64      `json:"playlist_count,omitempty"`
	Playlist      string      `json:"playlist,omitempty"`
	PlaylistID    string      `json:"playlist_id,omitempty"`
	PlaylistIndex *int        `json:"playlist_index,omitempty"`
	Entries       []*InfoDict `json:"entries,omitempty"`

	// Selected format (copied from the chosen Format during processing)
	FormatID   string  `json:"format_id,omitempty"`
	VCodec     string  `json:"vcodec,omitempty"`
	ACodec     string  `json:"acodec,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Resolution string  `json:"resolution,omitempty"`
	FPS        float64 `json:"fps,omitempty"`
	TBR        float64 `json:"tbr,omitempty"`

	// Output
	Filepath string `json:"filepath,omitempty"`

	// Comments (filled by post extractor)
	Comments []Comment `json:"comments,omitempty"`

	// Internal, never serialized as-is
	HTTPHeaders        map[string]string `json:"http_headers,omitempty"`
	Protocol           string            `json:"protocol,omitempty"`
	RequestedDownloads []Format          `json:"-"`
	FormatSortFields   []string          `json:"_format_sort_fields,omitempty"`
	HasDRM             any               `json:"has_drm,omitempty"`
	SkipDownload       bool              `json:"-"`
}

// Format describes a single downloadable stream.
type Format struct {
	URL         string `json:"url"`
	RequestData []byte `json:"request_data,omitempty"`

	FormatID   string `json:"format_id,omitempty"`
	Format     string `json:"format,omitempty"`
	FormatNote string `json:"format_note,omitempty"`
	Ext        string `json:"ext,omitempty"`

	Width         int      `json:"width,omitempty"`
	Height        int      `json:"height,omitempty"`
	Rows          int      `json:"rows,omitempty"`
	Columns       int      `json:"columns,omitempty"`
	Resolution    string   `json:"resolution,omitempty"`
	AspectRatio   *float64 `json:"aspect_ratio,omitempty"`
	FPS           float64  `json:"fps,omitempty"`
	TBR           float64  `json:"tbr,omitempty"`
	VBR           float64  `json:"vbr,omitempty"`
	ABR           float64  `json:"abr,omitempty"`
	ASR           int      `json:"asr,omitempty"`
	AudioChannels int      `json:"audio_channels,omitempty"`

	Filesize       int64 `json:"filesize,omitempty"`
	FilesizeApprox int64 `json:"filesize_approx,omitempty"`

	ACodec       string `json:"acodec,omitempty"`
	VCodec       string `json:"vcodec,omitempty"`
	DynamicRange string `json:"dynamic_range,omitempty"`
	Language     string `json:"language,omitempty"`
	Container    string `json:"container,omitempty"`
	Protocol     string `json:"protocol,omitempty"`

	Preference         *int     `json:"preference,omitempty"`
	Quality            *float64 `json:"quality,omitempty"`
	SourcePreference   *int     `json:"source_preference,omitempty"`
	LanguagePreference *int     `json:"language_preference,omitempty"`

	HTTPHeaders     map[string]string `json:"http_headers,omitempty"`
	Fragments       []Fragment        `json:"fragments,omitempty"`
	FragmentBaseURL string            `json:"fragment_base_url,omitempty"`

	HasDRM            any            `json:"has_drm,omitempty"`
	StretchedRatio    *float64       `json:"stretched_ratio,omitempty"`
	IsLive            *bool          `json:"is_live,omitempty"`
	ManifestURL       string         `json:"manifest_url,omitempty"`
	DownloaderOptions map[string]any `json:"downloader_options,omitempty"`

	// RequestedFormats holds the individual streams selected for a merged
	// download (bestvideo+bestaudio).
	RequestedFormats []Format `json:"requested_formats,omitempty"`

	// SelectedFormats is the full set of formats chosen by the selector
	// (internal, used to drive the download stage).
	SelectedFormats []Format `json:"-"`
}

// Fragment is one segment of a fragmented download.
type Fragment struct {
	URL      string  `json:"url,omitempty"`
	Path     string  `json:"path,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Filesize int64   `json:"filesize,omitempty"`
}

// Thumbnail is a still image of the media.
type Thumbnail struct {
	ID         string `json:"id,omitempty"`
	URL        string `json:"url"`
	Ext        string `json:"ext,omitempty"`
	Preference *int   `json:"preference,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Filesize   int64  `json:"filesize,omitempty"`
}

// Subtitle is one downloadable subtitle representation in one language.
type Subtitle struct {
	Ext         string            `json:"ext"`
	URL         string            `json:"url,omitempty"`
	Data        string            `json:"data,omitempty"`
	Name        string            `json:"name,omitempty"`
	HTTPHeaders map[string]string `json:"http_headers,omitempty"`
	Protocol    string            `json:"protocol,omitempty"`
}

// Chapter is a time range of the media.
type Chapter struct {
	StartTime float64  `json:"start_time"`
	EndTime   *float64 `json:"end_time,omitempty"`
	Title     string   `json:"title,omitempty"`
}

// HeatmapSegment is a "most liked moment" range.
type HeatmapSegment struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Value     float64 `json:"value"`
}

// Comment is a single user comment.
type Comment struct {
	Author    string `json:"author,omitempty"`
	AuthorID  string `json:"author_id,omitempty"`
	AuthorURL string `json:"author_url,omitempty"`
	ID        string `json:"id,omitempty"`
	Text      string `json:"text,omitempty"`
	HTML      string `json:"html,omitempty"`
	Timestamp *int64 `json:"timestamp,omitempty"`
	Parent    string `json:"parent,omitempty"`
	LikeCount *int64 `json:"like_count,omitempty"`
}

// IsPlaylist reports whether the dict describes a playlist.
func (d *InfoDict) IsPlaylist() bool {
	return d.Type == "playlist" || d.Type == "multi_video"
}

// IsURLResult reports whether the dict is a redirect to another extractor.
func (d *InfoDict) IsURLResult() bool {
	return d.Type == "url" || d.Type == "url_transparent"
}

// Clone shallow-copies the dict (entries/formats shared).
func (d *InfoDict) Clone() *InfoDict {
	c := *d
	return &c
}
