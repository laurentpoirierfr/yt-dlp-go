package postprocessor

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func TestAudioFormatCodec(t *testing.T) {
	cases := map[string]struct {
		codec    string
		reencode bool
	}{
		"mp3":    {"libmp3lame", true},
		"aac":    {"aac", true},
		"m4a":    {"aac", true},
		"opus":   {"libopus", true},
		"vorbis": {"libvorbis", true},
		"flac":   {"flac", true},
		"wav":    {"pcm_s16le", true},
		"wma":    {"wmav2", true},
		"best":   {"", false},
		"bogus":  {"", false},
	}
	for format, want := range cases {
		codec, re := AudioFormatCodec(format)
		if codec != want.codec || re != want.reencode {
			t.Errorf("AudioFormatCodec(%q) = (%q,%v), want (%q,%v)", format, codec, re, want.codec, want.reencode)
		}
	}
}

func TestAudioArgs(t *testing.T) {
	got := audioArgs("in.mp4", "out.mp3", "mp3", "3")
	want := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", "in.mp4", "-vn",
		"-acodec", "libmp3lame", "-q:a", "3", "out.mp3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("audioArgs mp3 = %v, want %v", got, want)
	}

	got = audioArgs("in.mp4", "out.m4a", "m4a", "6")
	if !contains(got, "-q:a") || !contains(got, "6") {
		t.Errorf("audioArgs m4a quality missing: %v", got)
	}

	got = audioArgs("in.m4a", "out.mp3", "mp3", "192K")
	if !contains(got, "-b:a") || !contains(got, "192K") {
		t.Errorf("audioArgs bitrate missing: %v", got)
	}

	got = audioArgs("in.mkv", "in.mkv.ytdlp.tmp", "best", "")
	if contains(got, "-acodec") || contains(got, "-q:a") || contains(got, "-b:a") {
		t.Errorf("audioArgs best should not re-encode: %v", got)
	}
}

func TestAudioDest(t *testing.T) {
	if dest, inPlace := audioDest("/d/Video [id].mkv", "mp3"); dest != "/d/Video [id].mp3" || inPlace {
		t.Errorf("audioDest mp3 = (%q,%v)", dest, inPlace)
	}
	if dest, inPlace := audioDest("/d/Video [id].m4a", "best"); dest != "/d/Video [id].ytdlp.init.m4a" || !inPlace {
		t.Errorf("audioDest best = (%q,%v)", dest, inPlace)
	}
}

func TestExtractAudioBestInPlace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shim test is POSIX only")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(src, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := filepath.Join(dir, "ffmpeg.sh")
	script := "#!/bin/sh\nlast=\nfor a in \"$@\"; do last=\"$a\"; done\nprintf 'audio' > \"$last\"\n"
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ex := NewFFmpegAudioExtractor(sh, "best", "", nil)
	dest, err := ex.Extract(src, &infodict.InfoDict{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if dest != src {
		t.Errorf("dest = %q, want in-place %q", dest, src)
	}
	if data, _ := os.ReadFile(src); string(data) != "audio" {
		t.Errorf("content = %q, want audio", string(data))
	}
	tmp, _ := audioDest(src, "best")
	if _, err := os.Stat(tmp); err == nil {
		t.Error("temporary file was not cleaned up")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
