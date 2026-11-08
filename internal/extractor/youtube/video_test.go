package youtube

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/extractor"
)

func prWithStatus(status map[string]any) []map[string]any {
	return []map[string]any{{"playabilityStatus": status}}
}

func msgOf(t *testing.T, err error) string {
	t.Helper()
	switch e := err.(type) {
	case *extractor.NoFormatsError:
		return e.Msg
	case *extractor.GeoRestrictedError:
		return e.Msg
	default:
		t.Fatalf("unexpected error type %T: %v", err, err)
		return ""
	}
}

func TestNoFormatsErrorReasons(t *testing.T) {
	ie := newTestIE()
	ie.SetDownloader(newHarness())

	cases := []struct {
		name    string
		prs     []map[string]any
		wantMsg string
	}{
		{
			name:    "unavailable",
			prs:     prWithStatus(map[string]any{"status": "ERROR", "reason": "This video is unavailable"}),
			wantMsg: "This video is unavailable",
		},
		{
			name: "error message renderer reason and subreason",
			prs: prWithStatus(map[string]any{
				"status": "ERROR",
				"reason": "Video unavailable",
				"errorScreen": map[string]any{
					"playerErrorMessageRenderer": map[string]any{
						"reason":    map[string]any{"runs": []any{map[string]any{"text": "Video unavailable"}}},
						"subreason": map[string]any{"runs": []any{map[string]any{"text": "This video is unavailable"}}},
					},
				},
			}),
			wantMsg: "Video unavailable. This video is unavailable",
		},
		{
			name: "sign in hint",
			prs: prWithStatus(map[string]any{
				"status": "LOGIN_REQUIRED",
				"reason": "Sign in to confirm you're not a bot. This helps protect our community. Learn more",
			}),
			wantMsg: "Sign in to confirm you're not a bot. " + youtubeLoginHint,
		},
		{
			name: "captcha challenge",
			prs: prWithStatus(map[string]any{
				"status":      "ERROR",
				"reason":      "Some reason",
				"errorScreen": map[string]any{"playerCaptchaViewModel": map[string]any{}},
			}),
			wantMsg: "Some reason. YouTube is requiring a captcha challenge before playback",
		},
		{
			name: "rate limited",
			prs: prWithStatus(map[string]any{
				"status": "ERROR",
				"reason": "This content isn't available, try again later",
			}),
			wantMsg: "This content isn't available, try again later. The current session " +
				"has been rate-limited by YouTube for up to an hour. It is recommended to use `-t sleep` to add a delay " +
				"between video requests to avoid exceeding the rate limit. For more information, refer to  " +
				"https://github.com/yt-dlp/yt-dlp/wiki/Extractors#this-content-isnt-available-try-again-later",
		},
		{
			name:    "drm protected",
			prs:     []map[string]any{{"streamingData": map[string]any{"licenseInfos": []any{map[string]any{}}}}},
			wantMsg: "This video is DRM protected",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ie.noFormatsError(c.prs, nil, "")
			if err == nil {
				t.Fatal("expected error")
			}
			if got := msgOf(t, err); got != c.wantMsg {
				t.Errorf("msg = %q\nwant  %q", got, c.wantMsg)
			}
		})
	}
}

func TestNoFormatsErrorGeoRestricted(t *testing.T) {
	ie := newTestIE()
	ie.SetDownloader(newHarness())
	prs := prWithStatus(map[string]any{
		"status": "ERROR",
		"errorScreen": map[string]any{
			"playerErrorMessageRenderer": map[string]any{
				"subreason": map[string]any{"runs": []any{
					map[string]any{"text": "The uploader has not made this video available in your country"},
				}},
			},
		},
	})
	err := ie.noFormatsError(prs, nil, "")
	geo, ok := err.(*extractor.GeoRestrictedError)
	if !ok {
		t.Fatalf("expected GeoRestrictedError, got %T: %v", err, err)
	}
	if geo.Msg != "The uploader has not made this video available in your country" {
		t.Errorf("msg = %q", geo.Msg)
	}
}

func TestInvalidPlayerResponse(t *testing.T) {
	cases := []struct {
		name    string
		pr      map[string]any
		videoID string
		want    string
	}{
		{"match", map[string]any{"videoDetails": map[string]any{"videoId": "abc"}}, "abc", ""},
		{"mismatch", map[string]any{"videoDetails": map[string]any{"videoId": "xyz"}}, "abc", "xyz"},
		{"missing id is valid", map[string]any{"videoDetails": map[string]any{}}, "abc", ""},
		{"no video details is valid", map[string]any{"playabilityStatus": map[string]any{}}, "abc", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := invalidPlayerResponse(c.pr, c.videoID); got != c.want {
				t.Errorf("invalidPlayerResponse = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRemoveEnd(t *testing.T) {
	if got := removeEnd("abc def", " def"); got != "abc" {
		t.Errorf("removeEnd = %q", got)
	}
	if got := removeEnd("abc", "xyz"); got != "abc" {
		t.Errorf("removeEnd = %q", got)
	}
	if got := removeEnd("abc", ""); got != "abc" {
		t.Errorf("removeEnd = %q", got)
	}
}
