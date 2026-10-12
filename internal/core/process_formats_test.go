package core

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/options"
)

func TestProcessFormatFields(t *testing.T) {
	y := New(options.DefaultOptions())
	formats := []infodict.Format{
		{FormatID: "140", URL: "https://x/audio.m4a", Ext: "m4a", VCodec: "none", ACodec: "mp4a.40.2", TBR: 129.796, ABR: ptrFloat(129.796), FormatNote: "medium"},
		{FormatID: "137", URL: "https://x/video.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none", Width: 1920, Height: 1080, TBR: 4000, FormatNote: "1080p"},
		{FormatID: "sb0", URL: "https://x/sb.jpg", Ext: "mhtml", VCodec: "none", ACodec: "none", Width: 320, Height: 180, FormatNote: "storyboard"},
		{FormatID: "426", URL: "https://x/v.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none", Width: 426, Height: 240},
		{FormatID: "bad id", URL: "https://x/v.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none"},
	}
	y.processFormatFields(formats, 100)

	audio := formats[0]
	if audio.Resolution != "audio only" || audio.DynamicRange != "" || audio.AspectRatio != nil ||
		audio.VideoExt != "none" || audio.AudioExt != "m4a" || audio.Format != "140 - audio only (medium)" {
		t.Errorf("audio format wrong: %+v", audio)
	}

	video := formats[1]
	if video.Resolution != "1920x1080" || video.DynamicRange != "SDR" ||
		video.VideoExt != "mp4" || video.AudioExt != "none" ||
		video.Format != "137 - 1920x1080 (1080p)" {
		t.Errorf("video format wrong: %+v", video)
	}
	if video.AspectRatio == nil || *video.AspectRatio != 1.78 {
		t.Errorf("video aspect_ratio = %v, want 1.78", video.AspectRatio)
	}

	sb := formats[2]
	if sb.Resolution != "320x180" || sb.Format != "sb0 - 320x180 (storyboard)" ||
		sb.VideoExt != "none" || sb.AudioExt != "none" {
		t.Errorf("storyboard format wrong: %+v", sb)
	}

	// Python round semantics: 426/240 = 1.775 -> 1.77 (not 1.78).
	if formats[3].AspectRatio == nil || *formats[3].AspectRatio != 1.77 {
		t.Errorf("426x240 aspect_ratio = %v, want 1.77", formats[3].AspectRatio)
	}

	if formats[4].FormatID != "bad_id" {
		t.Errorf("format_id = %q, want bad_id", formats[4].FormatID)
	}
}
