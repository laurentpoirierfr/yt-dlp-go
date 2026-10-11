package postprocessor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

// audioFormatExt maps --audio-format values to the output file extension.
var audioFormatExt = map[string]string{
	"best":   "",
	"aac":    "aac",
	"aiff":   "aiff",
	"alac":   "m4a",
	"flac":   "flac",
	"m4a":    "m4a",
	"mp3":    "mp3",
	"ogg":    "ogg",
	"opus":   "opus",
	"vorbis": "ogg",
	"wav":    "wav",
	"wma":    "wma",
}

// audioFormatCodec maps --audio-format values to the ffmpeg audio encoder.
// "best" keeps the original stream and is handled separately.
var audioFormatCodec = map[string]string{
	"aac":    "aac",
	"aiff":   "pcm_s16be",
	"alac":   "alac",
	"flac":   "flac",
	"m4a":    "aac",
	"mp3":    "libmp3lame",
	"ogg":    "libvorbis",
	"opus":   "libopus",
	"vorbis": "libvorbis",
	"wav":    "pcm_s16le",
	"wma":    "wmav2",
}

// AudioFormatCodec returns the encoder (and whether the stream is re-encoded)
// for an --audio-format value.
func AudioFormatCodec(format string) (codec string, reencode bool) {
	codec, ok := audioFormatCodec[format]
	return codec, ok
}

// audioArgs builds the ffmpeg command line for a conversion. quality is the
// --audio-quality value ("" = default). It is only applied when re-encoding.
func audioArgs(input, dest, format, quality string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", input, "-vn"}
	if codec, ok := audioFormatCodec[format]; ok {
		args = append(args, "-acodec", codec)
		if quality == "" {
			quality = "5"
		}
		if isDecimal(quality) {
			args = append(args, "-q:a", quality)
		} else {
			args = append(args, "-b:a", quality)
		}
	}
	args = append(args, dest)
	return args
}

// isDecimal reports whether s is a plain non-negative integer.
func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// audioTmpSuffix is inserted into the name of in-place conversions ("best"
// keeps the original container, so ffmpeg writes to a temporary file that is
// renamed over the source). The original extension is kept so ffmpeg can infer
// the output muxer.
const audioTmpSuffix = ".ytdlp.init"

// audioDest resolves the output path for a conversion (destination, and whether
// the conversion happens in place through a temporary file).
func audioDest(input, format string) (string, bool) {
	if format == "best" || audioFormatExt[format] == "" {
		base := strings.TrimSuffix(input, filepath.Ext(input))
		return base + audioTmpSuffix + filepath.Ext(input), true
	}
	dir := filepath.Dir(input)
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return filepath.Join(dir, base+"."+audioFormatExt[format]), false
}

// FFmpegAudioExtractor converts media files to audio-only (yt-dlp -x).
type FFmpegAudioExtractor struct {
	Location string
	Format   string
	Quality  string
	sink     events.Sink
}

// NewFFmpegAudioExtractor builds an audio extractor. A format of "" or "best"
// strips the video stream without re-encoding. sink may be nil.
func NewFFmpegAudioExtractor(location, format, quality string, sink events.Sink) *FFmpegAudioExtractor {
	if sink == nil {
		sink = events.NopSink{}
	}
	return &FFmpegAudioExtractor{Location: location, Format: format, Quality: quality, sink: sink}
}

func (e *FFmpegAudioExtractor) bin() string {
	if e.Location != "" {
		return e.Location
	}
	return "ffmpeg"
}

// Available reports whether ffmpeg can be found.
func (e *FFmpegAudioExtractor) Available() bool {
	_, err := exec.LookPath(e.bin())
	return err == nil
}

// Extract strips or re-encodes the audio of file into dest, removing the video
// stream. It returns the destination path.
func (e *FFmpegAudioExtractor) Extract(input string, info *infodict.InfoDict) (string, error) {
	if e.Format == "" {
		e.Format = "best"
	}
	dest, inPlace := audioDest(input, e.Format)
	e.sink.PostProcess(events.PostProcess{Info: info, Name: "AudioConvert", Status: events.StatusStarted})
	args := audioArgs(input, dest, e.Format, e.Quality)
	cmd := exec.Command(e.bin(), args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		werr := fmt.Errorf("ffmpeg audio extraction failed: %v: %s", err, strings.TrimSpace(stderr.String()))
		e.sink.PostProcess(events.PostProcess{Info: info, Name: "AudioConvert", Status: events.StatusError, Err: werr})
		return "", werr
	}
	e.sink.PostProcess(events.PostProcess{Info: info, Name: "AudioConvert", Status: events.StatusFinished})
	if inPlace {
		if err := os.Rename(dest, input); err != nil {
			return "", err
		}
		dest = input
	}
	return dest, nil
}

// OutputExt returns the final file extension for the configured format.
func (e *FFmpegAudioExtractor) OutputExt() string {
	return audioFormatExt[e.Format]
}
