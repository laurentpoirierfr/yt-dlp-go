package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

// unsupportedFlags lists flags that are accepted for yt-dlp compatibility but
// whose behaviour is not implemented yet. Setting one explicitly is reported as
// an error instead of being silently ignored.
var unsupportedFlags = map[string]string{
	// Post-processing
	"remux-video": "remuxing is not implemented yet",

	// Format selection extras
	"allow-unplayable-formats": "unsupported format override is not implemented",
	"ignore-no-formats-error":  "no-format fallback is not implemented",
	"check-formats":            "format checking is not implemented",

	// Extraction
	"force-generic-extractor": "forcing the generic extractor is not implemented",

	// Error handling / control flow
	"fragment-retries":     "fragmented downloads are not implemented",
	"extractor-retries":    "extractor retries are not implemented",
	"limit-rate":           "rate limiting is not implemented",
	"http-chunk-size":      "chunked HTTP downloads are not implemented",
	"concurrent-fragments": "fragmented downloads are not implemented",

	// Playlist ordering
	"break-on-reject": "entry filtering is not implemented",
}

// validateConvertSubtitles normalizes and validates the --convert-subs value.
func validateConvertSubtitles(o *options.Options) error {
	v := strings.ToLower(strings.TrimSpace(o.ConvertSubtitles))
	switch v {
	case "", "none":
		o.ConvertSubtitles = ""
		return nil
	case "srt", "vtt":
		o.ConvertSubtitles = v
		return nil
	default:
		return fmt.Errorf("--convert-subs: unsupported format %q (supported: srt, vtt)", o.ConvertSubtitles)
	}
}

// mergeOutputFormatRE mirrors yt-dlp's validation of --merge-output-format
// (re.match, i.e. a prefix match: one common video container, optionally
// followed by "/" and more containers).
var mergeOutputFormatRE = regexp.MustCompile(`^(avi|flv|mkv|mov|mp4|webm)(/(avi|flv|mkv|mov|mp4|webm))*`)

// validateMergeOutputFormat ports yt-dlp's validate_regex call for
// --merge-output-format. It only validates when the flag was explicitly set
// (yt-dlp skips validation for its None default).
func validateMergeOutputFormat(cmd *cobra.Command, o *options.Options) error {
	if !cmd.Flags().Changed("merge-output-format") {
		return nil
	}
	if !mergeOutputFormatRE.MatchString(o.MergeOutputFormat) {
		return fmt.Errorf("invalid merge output format %q given", o.MergeOutputFormat)
	}
	return nil
}

// validateUnsupported returns an error if the user explicitly set one or more
// flags whose behaviour is not implemented.
func validateUnsupported(cmd *cobra.Command) error {
	var problems []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if reason, ok := unsupportedFlags[f.Name]; ok {
			problems = append(problems, fmt.Sprintf("  --%s: %s", f.Name, reason))
		}
	})
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("unsupported option(s):\n%s", strings.Join(problems, "\n"))
}
