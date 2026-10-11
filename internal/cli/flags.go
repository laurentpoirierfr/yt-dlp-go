package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func newRootCmd(o *options.Options) *cobra.Command {
	var (
		simulate     bool
		ignoreErrors bool
		flatPlaylist bool
		writeThumb   bool
		allThumbs    bool
		continueDL   bool
		addHeaders   []string
		forceOver    bool
		yesPlaylist  bool
		outTmpl      = o.Outtmpl["default"]
	)

	cmd := &cobra.Command{
		Use:           "yt-dlp-go [OPTIONS] URL [URL...]",
		Short:         "A Go port of yt-dlp",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE:          func(cmd *cobra.Command, args []string) error { return nil },
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if o.Outtmpl == nil {
				o.Outtmpl = map[string]string{}
			}
			o.Outtmpl["default"] = outTmpl
			s := simulate
			o.Simulate = &s
			if ignoreErrors {
				o.IgnoreErrors = options.IgnoreAll
			}
			if flatPlaylist {
				o.ExtractFlat = "in_playlist"
			}
			if writeThumb {
				o.WriteThumbnail = options.ThumbnailBest
			}
			if allThumbs {
				o.WriteThumbnail = options.ThumbnailAll
			}
			if cmd.Flags().Changed("continue") {
				o.ContinueDL = &continueDL
			}
			if cmd.Flags().Changed("force-overwrites") {
				v := forceOver
				o.Overwrites = &v
			}
			if yesPlaylist {
				o.NoPlaylist = false
			}
			if o.HTTPHeaders == nil {
				o.HTTPHeaders = map[string]string{}
			}
			for _, h := range addHeaders {
				if k, v, ok := strings.Cut(h, ":"); ok {
					o.HTTPHeaders[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
			}
			if err := validateUnsupported(cmd); err != nil {
				return err
			}
			return validateConvertSubtitles(o)
		},
	}

	f := cmd.Flags()

	// Output / format selection
	f.StringVarP(&outTmpl, "output", "o", outTmpl, "Output filename template")
	f.StringVarP(&o.Format, "format", "f", o.Format, "Format selection")
	f.StringSliceVarP(&o.FormatSort, "format-sort", "S", nil, "Sort the formats by the fields given")
	f.BoolVar(&o.FormatSortForce, "format-sort-force", false, "Force user specified sort order to have precedence")
	f.BoolVar(&o.PreferFreeFormats, "prefer-free-formats", false, "Prefer free video formats")
	f.StringVar(&o.MergeOutputFormat, "merge-output-format", o.MergeOutputFormat, "Container format for merging")
	f.StringVar(&o.FinalExt, "remux-video", o.FinalExt, "Remux the video into another container")
	f.BoolVar(&o.AllowUnplayableFormats, "allow-unplayable-formats", false, "Allow unsafe, unplayable formats")
	f.BoolVar(&o.IgnoreNoFormatsError, "ignore-no-formats-error", false, "Ignore \"No video formats\" error")
	f.StringVar(&o.CheckFormats, "check-formats", o.CheckFormats, "Check that the formats are downloadable")

	// Filesystem
	f.StringToStringVarP(&o.Paths, "paths", "P", nil, "Paths where to save files (home:, temp:)")
	f.BoolVar(&o.RestrictFilenames, "restrict-filenames", false, "Restrict filenames to ASCII")
	f.BoolVar(&o.WindowsFilenames, "windows-filenames", false, "Force filenames to be Windows-compatible")
	f.IntVar(&o.TrimFileName, "trim-filenames", o.TrimFileName, "Limit the filename length")
	f.BoolVar(&o.UseID, "id", false, "Use only video ID in file name")
	f.BoolVar(&o.NoPart, "no-part", false, "Do not use .part files")
	f.BoolVar(&o.ForceGenericExtractor, "force-generic-extractor", false, "Force generic extractor")

	// Verbosity / simulation
	f.BoolVarP(&o.Quiet, "quiet", "q", false, "Suppress console output")
	f.BoolVar(&o.NoWarnings, "no-warnings", false, "Suppress warnings")
	f.BoolVarP(&o.Verbose, "verbose", "v", false, "Print various debugging information")
	f.BoolVar(&o.TUI, "tui", false, "Use the interactive terminal interface")
	f.BoolVar(&o.NoTUI, "no-tui", false, "Disable the interactive terminal interface")
	f.BoolVarP(&simulate, "simulate", "s", false, "Do not download the video")
	f.BoolVar(&o.SkipDownload, "skip-download", false, "Do not download the video")
	f.BoolVar(&o.NoProgress, "no-progress", false, "Do not print the progress bar")
	f.BoolVar(&o.ForceURL, "get-url", false, "Simulate and print the URL")
	f.BoolVar(&o.ForceTitle, "get-title", false, "Simulate and print the title")
	f.BoolVar(&o.ForceID, "get-id", false, "Simulate and print the id")
	f.BoolVar(&o.ForceFilename, "get-filename", false, "Simulate and print the filename")
	f.BoolVarP(&o.ForceJSON, "dump-json", "j", false, "Quiet, but print JSON information per video")
	f.BoolVarP(&o.DumpSingleJSON, "dump-single-json", "J", false, "Quiet, but print JSON information per URL")
	f.BoolVarP(&o.ListFormats, "list-formats", "F", false, "List available formats of each video")

	// Error handling
	f.BoolVarP(&ignoreErrors, "ignore-errors", "i", false, "Continue on download errors")
	f.BoolVar(&o.BreakOnExisting, "break-on-existing", false, "Stop when encountering a file in the archive")
	f.BoolVar(&o.BreakOnReject, "break-on-reject", false, "Stop when encountering a filtered video")
	f.BoolVar(&o.BreakPerURL, "break-per-input", false, "Break on first error per input")

	// Networking
	f.StringVar(&o.Proxy, "proxy", o.Proxy, "Use the specified HTTP/HTTPS/SOCKS proxy")
	f.DurationVar(&o.SocketTimeout, "socket-timeout", o.SocketTimeout, "Time to wait before giving up, in seconds")
	f.StringVar(&o.UserAgent, "user-agent", o.UserAgent, "A custom User-Agent header")
	f.StringArrayVar(&addHeaders, "add-header", nil, "Specify a custom HTTP header")
	f.BoolVar(&o.NoCheckCertificate, "no-check-certificate", false, "Suppress HTTPS certificate validation")
	f.IntVar(&o.Retries, "retries", o.Retries, "Retry on download error")
	f.IntVar(&o.FragmentRetries, "fragment-retries", o.FragmentRetries, "Retries for a fragment")
	f.IntVar(&o.ExtractorRetries, "extractor-retries", o.ExtractorRetries, "Retries for known extractor errors")
	f.Int64Var(&o.RateLimit, "limit-rate", o.RateLimit, "Maximum download rate")
	f.Int64Var(&o.HTTPChunkSize, "http-chunk-size", o.HTTPChunkSize, "Size of a chunk for HTTP downloading")
	f.IntVar(&o.ConcurrentFragmentDownloads, "concurrent-fragments", o.ConcurrentFragmentDownloads, "Fragments to download simultaneously")

	// Filesystem behaviour
	f.BoolVar(&continueDL, "continue", false, "Resume partially downloaded files")
	f.BoolVar(&forceOver, "force-overwrites", false, "Overwrite all video and metadata files")
	f.StringVar(&o.DownloadArchive, "download-archive", o.DownloadArchive, "Download videos listed in file, and write each downloaded one to it")

	// Playlist
	f.BoolVar(&o.NoPlaylist, "no-playlist", false, "Download only the video if a URL refers to both")
	f.BoolVar(&yesPlaylist, "yes-playlist", false, "Download the playlist, if the URL refers to a video and a playlist")
	f.IntVar(&o.PlaylistStart, "playlist-start", o.PlaylistStart, "Playlist video to start at")
	f.IntVar(&o.PlaylistEnd, "playlist-end", o.PlaylistEnd, "Playlist video to end at")
	f.StringVarP(&o.PlaylistItems, "playlist-items", "I", o.PlaylistItems, "Playlist video items to download")
	f.BoolVar(&o.PlaylistReverse, "playlist-reverse", false, "Download playlist videos in reverse order")
	f.BoolVar(&o.PlaylistRandom, "playlist-random", false, "Download playlist videos in random order")
	f.BoolVar(&flatPlaylist, "flat-playlist", false, "Do not extract the videos of a playlist")

	// Metadata / subtitles
	f.BoolVar(&writeThumb, "write-thumbnail", false, "Write thumbnail image to disk")
	f.BoolVar(&allThumbs, "write-all-thumbnails", false, "Write all thumbnail image formats to disk")
	f.BoolVar(&o.ListThumbnails, "list-thumbnails", false, "Simulate and list all available thumbnail formats")
	f.BoolVar(&o.WriteInfoJSON, "write-info-json", false, "Write video metadata to a .info.json file")
	f.BoolVar(&o.WriteDescription, "write-description", false, "Write video description to a .description file")
	f.BoolVar(&o.WriteSubtitles, "write-subs", false, "Write subtitle file")
	f.BoolVar(&o.WriteAutoSubs, "write-auto-subs", false, "Write automatically generated subtitle file")
	f.BoolVar(&o.AllSubtitles, "all-subs", false, "Download all the available subtitles")
	f.BoolVar(&o.ListSubtitles, "list-subs", false, "List available subtitles of each video")
	f.StringSliceVar(&o.SubtitlesLangs, "sub-langs", nil, "Languages of the subtitles to download (can be regex) or \"all\"")
	f.StringVar(&o.SubtitlesFormat, "sub-format", o.SubtitlesFormat, "Subtitle format preference separated by \"/\"")
	f.StringVar(&o.ConvertSubtitles, "convert-subs", o.ConvertSubtitles, "Convert the subtitles to another format (srt, vtt)")
	f.StringVar(&o.ConvertSubtitles, "convert-subtitles", o.ConvertSubtitles, "Alias of --convert-subs")

	f.StringVar(&o.FFmpegLocation, "ffmpeg-location", o.FFmpegLocation, "Location of the ffmpeg binary")

	f.BoolVarP(&o.ExtractAudio, "extract-audio", "x", false, "Convert video files to audio-only files")
	f.StringVar(&o.AudioFormat, "audio-format", o.AudioFormat, "Best audio format to be used when -x is used (best, mp3, aac, m4a, opus, flac, wav...)")
	f.StringVar(&o.AudioQuality, "audio-quality", o.AudioQuality, "Specify ffmpeg audio quality, a value between 0 (best) and 9 (worst), or a bitrate")
	f.BoolVarP(&o.KeepVideo, "keep-video", "k", false, "Keep the video file on disk after post-processing")

	return cmd
}
