// Package options holds the runtime options of yt-dlp-go, equivalent to the
// `ydl_opts` / `params` dict of yt-dlp's YoutubeDL.
package options

import "time"

// Options mirrors the parameters accepted by yt-dlp's YoutubeDL(params=...).
type Options struct {
	// Credentials
	Username           string
	Password           string
	TwoFactor          string
	VideoPassword      string
	UseNetrc           bool
	NetrcLocation      string
	APUsername         string
	APPassword         string

	// Console / verbosity / simulation
	Quiet              bool
	NoWarnings         bool
	ForceURL           bool
	ForceTitle         bool
	ForceID            bool
	ForceThumbnail     bool
	ForceDescription   bool
	ForceDuration      bool
	ForceFilename      bool
	ForceFormat        bool
	ForceJSON          bool
	DumpSingleJSON     bool
	Simulate           *bool
	SkipDownload       bool
	Verbose            bool
	NoProgress         bool
	ProgressWithNewline bool
	Noprogress         bool

	// Error handling / control flow
	IgnoreErrors       IgnoreErrorsMode
	MaxDownloads       int
	BreakOnExisting    bool
	BreakOnReject      bool
	BreakPerURL        bool

	// Format selection
	Format             string
	FormatSort         []string
	FormatSortForce    bool
	CheckFormats       string // "", "selected", "all"
	ListFormats        bool
	ListFormatsTable   bool
	PreferFreeFormats  bool
	FinalExt           string
	MergeOutputFormat  string
	AllowUnplayableFormats bool
	IgnoreNoFormatsError   bool
	AllowMultipleVideoStreams bool
	AllowMultipleAudioStreams bool

	// Output templates / filesystem
	Outtmpl            map[string]string
	OuttmplNAPlaceholder string
	Paths              map[string]string
	RestrictFilenames  bool
	WindowsFilenames   bool
	TrimFileName       int
	UseID              bool
	NoPart             bool
	Overwrites         *bool
	ContinueDL         *bool
	Encoding           string
	CacheDir           string
	DownloadArchive    string
	ForceGenericExtractor bool

	// Metadata files / sidecars
	WriteDescription   bool
	WriteInfoJSON      bool
	CleanInfoJSON      *bool
	WriteThumbnail     ThumbnailMode
	ListThumbnails    bool
	GetComments        bool

	// Subtitles
	WriteSubtitles       bool
	WriteAutoSubs        bool
	AllSubtitles         bool
	ListSubtitles        bool
	SubtitlesFormat      string
	SubtitlesLangs       []string

	// Playlist / entry filtering
	PlaylistStart     int
	PlaylistEnd       int
	PlaylistItems     string
	PlaylistReverse   bool
	PlaylistRandom    bool
	NoPlaylist        bool
	ExtractFlat       string // "", "in_playlist", "discard", "discard_in_playlist"
	MatchTitle        string
	RejectTitle       string
	Date              string
	DateBefore        string
	DateAfter         string
	MinViews          int64
	MaxViews          int64
	MinFilesize       int64
	MaxFilesize       int64
	AgeLimit          int
	AllowedExtractors []string

	// Networking / retries / rate limits
	RateLimit            int64
	ThrottledRateLimit   int64
	Retries              int
	FileAccessRetries    int
	FragmentRetries      int
	ExtractorRetries     int
	SkipUnavailableFragments bool
	KeepFragments        bool
	ConcurrentFragmentDownloads int
	BufferSize           int64
	HTTPChunkSize        int64
	Test                 bool
	HTTPHeaders          map[string]string
	Proxy                string
	SocketTimeout        time.Duration
	NoCheckCertificate   bool
	PreferInsecure       bool
	SleepIntervalRequests float64
	SleepInterval        float64
	MaxSleepInterval     float64
	SleepIntervalSubtitles float64
	GeoBypass            string
	GeoBypassCountry     string
	GeoBypassIPBlock     string

	// Extraction / site behaviour
	DefaultSearch     string
	DynamicMPD        bool
	ExtractorArgs     map[string][]string
	LiveFromStart     bool
	WaitingForVideo   *[2]float64
	MarkWatched       bool

	// Download ranges / external tools
	ExternalDownloader      map[string]string
	ExternalDownloaderArgs  map[string][]string
	FFmpegLocation          string
	PostprocessorArgs       map[string][]string
	Postprocessors          []PostprocessorSpec
	Fixup                   string
	KeepVideo               bool

	// User agent / referer (folded into HTTPHeaders by the CLI)
	UserAgent string
	Referer   string
}

// IgnoreErrorsMode mirrors yt-dlp's tri-state ignoreerrors.
type IgnoreErrorsMode int

const (
	// IgnoreNone aborts on the first error (abort-on-error).
	IgnoreNone IgnoreErrorsMode = iota
	// IgnoreAll continues past errors.
	IgnoreAll
	// IgnoreOnlyDownload continues past errors only during download.
	IgnoreOnlyDownload
)

// ThumbnailMode mirrors yt-dlp's writethumbnail (false/true/"all").
type ThumbnailMode int

const (
	ThumbnailNone ThumbnailMode = iota
	ThumbnailBest
	ThumbnailAll
)

// PostprocessorSpec is a serialized postprocessor request
// (equivalent of yt-dlp's `params['postprocessors']` entries).
type PostprocessorSpec struct {
	Key    string
	When   string
	Options map[string]any
}

// DefaultOptions returns the yt-dlp compatible defaults.
func DefaultOptions() *Options {
	o := &Options{
		Quiet:                    false,
		NoWarnings:               false,
		IgnoreErrors:             IgnoreOnlyDownload,
		ListFormatsTable:         true,
		SubtitlesFormat:          "best",
		PlaylistStart:            1,
		Retries:                  10,
		FileAccessRetries:        3,
		FragmentRetries:           10,
		ExtractorRetries:         3,
		SkipUnavailableFragments: true,
		ConcurrentFragmentDownloads: 1,
		BufferSize:               1024,
		DynamicMPD:               true,
		GeoBypass:                "default",
		OuttmplNAPlaceholder:     "NA",
		Outtmpl:                  map[string]string{"default": "%(title)s [%(id)s].%(ext)s"},
		ExtractorArgs:            map[string][]string{},
		PostprocessorArgs:        map[string][]string{},
		AllowedExtractors:        []string{"default"},
	}
	return o
}

// GetExtractorArg returns the values for `--extractor-args IE:key=value`.
func (o *Options) GetExtractorArg(ie, key string) ([]string, bool) {
	vals, ok := o.ExtractorArgs[ie+":"+key]
	return vals, ok
}
