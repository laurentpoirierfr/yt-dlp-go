// Package extractor provides the extractor contract (InfoExtractor base),
// the registry and the shared result builders.
package extractor

import (
	"fmt"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

// Downloader is the interface extractors use to reach the core pipeline
// (equivalent of holding a reference to YoutubeDL).
type Downloader interface {
	Client() *networking.Client
	Options() *options.Options
	AllExtractors() []IE
	ToScreen(format string, args ...any)
	ReportWarning(format string, args ...any)
	ReportError(format string, args ...any)
	WriteDebug(format string, args ...any)
}

// IE is the extractor interface, mirroring yt-dlp's InfoExtractor.
type IE interface {
	IEKey() string
	IEName() string
	Suitable(url string) bool
	Working() bool
	SetDownloader(d Downloader)
	Downloader() Downloader
	Initialize() error
	RealExtract(url string) (*infodict.InfoDict, error)
	Extract(url string) (*infodict.InfoDict, error)
}

// ExtractorError is the port of yt-dlp's ExtractorError.
type ExtractorError struct {
	Msg      string
	VideoID  string
	IE       string
	Expected bool // raised by raise_no_formats: "no formats" rather than a bug
	Cause    error
}

// bugReportsMessage mirrors yt-dlp's _utils.bug_reports_message (default
// before=';'), byte-for-byte.
const bugReportsMessage = "; please report this issue on  https://github.com/yt-dlp/yt-dlp/issues?q= , filling out the appropriate issue template. Confirm you are on the latest version using  yt-dlp -U"

// formatField mirrors yt-dlp's utils.format_field for the string forms used
// in error message rendering: absent values render as nothing.
func formatField(s, tmpl string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf(tmpl, s)
}

// causeRepr produces a Python %r-like representation of a cause, so that
// " (caused by %r)" matches yt-dlp: network HTTP errors render as
// "<HTTPError <code>: <status>>", anything else as a quoted string.
func causeRepr(cause error) string {
	if he, ok := cause.(*networking.HTTPError); ok {
		return fmt.Sprintf("<HTTPError %d: %s>", he.StatusCode, he.Status)
	}
	return fmt.Sprintf("%q", cause.Error())
}

func (e *ExtractorError) Error() string {
	msg := formatField(e.IE, "[%s] ") + formatField(e.VideoID, "%s: ") + e.Msg
	if e.Cause != nil {
		msg += fmt.Sprintf(" (caused by %s)", causeRepr(e.Cause))
	}
	if !e.Expected {
		msg += bugReportsMessage
	}
	return msg
}

func (e *ExtractorError) Unwrap() error { return e.Cause }

// NewExtractorError builds an ExtractorError bound to the given IE/video.
func (e *Base) NewExtractorError(msg string, expected bool, cause error) *ExtractorError {
	return &ExtractorError{Msg: msg, VideoID: e.videoID, IE: e.IEKey(), Expected: expected, Cause: cause}
}

// GeoRestrictedError is the port of yt-dlp's GeoRestrictedError.
type GeoRestrictedError struct {
	Msg       string
	Countries []string
	IE        string
	VideoID   string
}

func (e *GeoRestrictedError) Error() string {
	return formatField(e.IE, "[%s] ") + formatField(e.VideoID, "%s: ") + e.Msg
}

// UnsupportedURLError is raised by UnsupportedURLIE.
type UnsupportedURLError struct{ URL string }

func (e *UnsupportedURLError) Error() string {
	return fmt.Sprintf("Unsupported URL: %s", e.URL)
}

// NoFormatsError is raised when an extractor found no downloadable formats.
type NoFormatsError struct {
	Msg      string
	Expected bool
	VideoID  string
	IE       string
}

func (e *NoFormatsError) Error() string {
	msg := formatField(e.IE, "[%s] ") + formatField(e.VideoID, "%s: ") + e.Msg
	if !e.Expected {
		msg += bugReportsMessage
	}
	return msg
}

// RegexNotFoundError is raised when a required pattern doesn't match.
type RegexNotFoundError struct{ Msg string }

func (e *RegexNotFoundError) Error() string { return e.Msg }

// IsExpected reports whether an error is an "expected" extraction failure
// (as opposed to an internal error).
func IsExpected(err error) bool {
	switch e := err.(type) {
	case *ExtractorError:
		return e.Expected
	case *GeoRestrictedError:
		return true
	case *UnsupportedURLError:
		return true
	case *NoFormatsError:
		return e.Expected
	}
	return false
}
