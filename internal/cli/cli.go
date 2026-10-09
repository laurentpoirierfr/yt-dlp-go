// Package cli implements the yt-dlp-go command line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elfeo/yt-dlp-go/internal/core"
	"github.com/elfeo/yt-dlp-go/internal/downloader"
	"github.com/elfeo/yt-dlp-go/internal/extractor"
	"github.com/elfeo/yt-dlp-go/internal/infodict"
	"github.com/elfeo/yt-dlp-go/internal/options"
	"github.com/elfeo/yt-dlp-go/internal/postprocessor"
)

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

	httpDL := downloader.NewHTTP(ydl.Client(), opts)
	var exit int
	ydl.SetDownloader(func(info *infodict.InfoDict, formats []infodict.Format) error {
		if opts.ListFormats {
			listFormats(info)
			return nil
		}
		if opts.ForceJSON || opts.DumpSingleJSON {
			return nil
		}
		if err := httpDL.Download(info, formats); err != nil {
			exit = 1
			return err
		}
		if len(info.RequestedDownloads) > 1 {
			merge(info, opts)
		}
		return nil
	})

	for _, url := range urls {
		simulate := opts.ListFormats || opts.ForceJSON || opts.DumpSingleJSON ||
			opts.ForceURL || opts.ForceTitle || opts.ForceID || opts.ForceFilename ||
			opts.SkipDownload
		info, err := ydl.ExtractInfo(url, !simulate)
		if err != nil {
			reportErr(err)
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
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
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

func merge(info *infodict.InfoDict, opts *options.Options) {
	merger := postprocessor.NewFFmpegMerger(opts.FFmpegLocation)
	var inputs []string
	for i := range info.RequestedDownloads {
		inputs = append(inputs, fmt.Sprintf("%s.f%d.%s", info.Filepath, i, orExt(info.RequestedDownloads[i].Ext)))
	}
	if err := merger.Merge(inputs, info.Filepath); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
		return
	}
	for _, in := range inputs {
		_ = os.Remove(in)
	}
	fmt.Fprintf(os.Stderr, "[Merger] Merged formats to \"%s\"\n", info.Filepath)
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

func reportErr(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
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

var _ = strings.TrimSpace
