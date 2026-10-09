// Package extractor provides the extractor contract (InfoExtractor base),
// the registry and the shared result builders.
package extractor

import (
	"fmt"

	"github.com/elfeo/yt-dlp-go/internal/infodict"
	"github.com/elfeo/yt-dlp-go/internal/networking"
	"github.com/elfeo/yt-dlp-go/internal/options"
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

func (e *ExtractorError) Error() string {
	msg := e.Msg
	if e.IE != "" {
		msg = fmt.Sprintf("[%s] %s", e.IE, msg)
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
}

func (e *GeoRestrictedError) Error() string { return e.Msg }

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
}

func (e *NoFormatsError) Error() string { return e.Msg }

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
