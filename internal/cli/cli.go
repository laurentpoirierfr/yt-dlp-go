// Package cli implements the yt-dlp-go command line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/core"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/downloader"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/postprocessor"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/tui"
)

// Version is the build version. It is injected at release time via -ldflags.
var Version = "dev"

// Main runs the CLI with the given extractors and arguments, returning an exit
// code.
func Main(ies []extractor.IE, args []string) int {
	opts := options.DefaultOptions()
	root := newRootCmd(opts)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return 1
	}
	// Help/usage or version was requested; nothing left to do.
	if help, _ := root.Flags().GetBool("help"); help {
		return 0
	}
	if version, _ := root.Flags().GetBool("version"); version {
		return 0
	}
	return run(ies, opts, root)
}

func run(ies []extractor.IE, opts *options.Options, root *cobra.Command) int {
	urls := root.Flags().Args()
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: You must provide at least one URL")
		return 1
	}

	ydl := core.New(opts)
	ydl.RegisterExtractors(ies)

	simulate := (opts.Simulate != nil && *opts.Simulate) || opts.ListFormats ||
		opts.ForceJSON || opts.DumpSingleJSON ||
		opts.ForceURL || opts.ForceTitle || opts.ForceID || opts.ForceFilename ||
		opts.SkipDownload

	process := func() int {
		sink := ydl.Sink()
		httpDL := downloader.NewHTTP(ydl.Client(), opts, sink)
		exit := 0
		ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error {
			if err := httpDL.Download(info, formats); err != nil {
				exit = 1
				return err
			}
			if len(info.RequestedDownloads) > 1 {
				merge(info, opts, sink)
			}
			return nil
		})

		for _, url := range urls {
			info, err := ydl.ExtractInfo(url, !simulate)
			if err != nil {
				sink.Log(events.Log{Level: events.LevelError, Text: err.Error()})
				exit = 1
				continue
			}
			if info == nil {
				continue
			}
			if opts.ListFormats {
				listFormats(info)
				continue
			}
			if opts.ForceJSON || opts.DumpSingleJSON {
				if err := dumpJSON(info); err != nil {
					sink.Log(events.Log{Level: events.LevelError, Text: err.Error()})
					exit = 1
				}
				continue
			}
			if opts.ForceURL {
				fmt.Println(info.URL)
			} else if opts.ForceTitle {
				fmt.Println(info.Title)
			} else if opts.ForceID {
				fmt.Println(info.ID)
			}
			if opts.ForceFilename && info.Filepath != "" {
				fmt.Println(info.Filepath)
			}
		}
		return exit
	}

	// The TUI is used automatically for real downloads on an interactive
	// terminal; --tui forces it, --no-tui (or -q) disables it.
	useTUI := !opts.NoTUI && !opts.Quiet && !simulate && tui.Supported()
	if useTUI {
		err := tui.Run(func(sink events.Sink) error {
			ydl.SetSink(sink)
			if code := process(); code != 0 {
				return fmt.Errorf("one or more downloads failed")
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			return 1
		}
		return 0
	}
	return process()
}

func merge(info *infodict.InfoDict, opts *options.Options, sink events.Sink) {
	merger := postprocessor.NewFFmpegMerger(opts.FFmpegLocation, sink)
	var inputs []string
	for i := range info.RequestedDownloads {
		inputs = append(inputs, fmt.Sprintf("%s.f%d.%s", info.Filepath, i, orExt(info.RequestedDownloads[i].Ext)))
	}
	if err := merger.Merge(inputs, info.Filepath, info); err != nil {
		sink.Log(events.Log{Level: events.LevelWarning, Text: err.Error()})
		return
	}
	for _, in := range inputs {
		_ = os.Remove(in)
	}
}

func orExt(e string) string {
	if e == "" {
		return "dat"
	}
	return e
}

func dumpJSON(info *infodict.InfoDict) error {
	enc := json.NewEncoder(os.Stdout)
	if os.Getenv("YTDL_JSON_PRETTY") != "" {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(info)
}

func listFormats(info *infodict.InfoDict) {
	fmt.Fprintf(os.Stdout, "[info] Available formats for %s:\n", info.ID)
	fmt.Fprintf(os.Stdout, "%-12s %-10s %-14s %-10s %-10s %s\n",
		"ID", "EXT", "RESOLUTION", "FPS", "VCODEC", "ACODEC")
	for _, f := range info.Formats {
		res := f.Resolution
		if res == "" {
			res = formatRes(f.Width, f.Height)
		}
		fmt.Fprintf(os.Stdout, "%-12s %-10s %-14s %-10.0f %-10s %s\n",
			f.FormatID, f.Ext, res, f.FPS, orNone(f.VCodec), orNone(f.ACodec))
	}
}

func formatRes(w, h int) string {
	switch {
	case w > 0 && h > 0:
		return fmt.Sprintf("%dx%d", w, h)
	case h > 0:
		return fmt.Sprintf("%dp", h)
	case w > 0:
		return fmt.Sprintf("%dx?", w)
	}
	return "audio only"
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
