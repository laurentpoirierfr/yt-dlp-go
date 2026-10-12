package extractor

import (
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/networking"
)

func TestExtractorErrorRendering(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "expected format-selection error",
			err: &ExtractorError{
				Msg:      "Requested format is not available. Use --list-formats for a list of available formats",
				VideoID:  "jNQXAC9IVRw",
				IE:       "youtube",
				Expected: true,
			},
			want: "[youtube] jNQXAC9IVRw: Requested format is not available. Use --list-formats for a list of available formats",
		},
		{
			name: "network cause renders as yt-dlp repr",
			err: &ExtractorError{
				Msg:      "Unable to download webpage: HTTP Error 404: Not Found",
				VideoID:  "xyz",
				IE:       "generic",
				Expected: true,
				Cause:    &networking.HTTPError{StatusCode: 404, Status: "Not Found"},
			},
			want: "[generic] xyz: Unable to download webpage: HTTP Error 404: Not Found (caused by <HTTPError 404: Not Found>)",
		},
		{
			name: "no prefix when ie and video id are empty",
			err: &ExtractorError{
				Msg:      "boom",
				Expected: true,
			},
			want: "boom",
		},
		{
			name: "bug report footer appended for unexpected errors",
			err: &ExtractorError{
				Msg:     "All player responses are invalid. Your IP is likely being blocked by Youtube",
				VideoID: "BaW_jenrAa2",
				IE:      "youtube",
			},
			want: "[youtube] BaW_jenrAa2: All player responses are invalid. Your IP is likely being blocked by Youtube" + bugReportsMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNoFormatsErrorRendering(t *testing.T) {
	err := &NoFormatsError{
		Msg:      "Requested format is not available. Use --list-formats for a list of available formats",
		VideoID:  "jNQXAC9IVRw",
		IE:       "youtube",
		Expected: true,
	}
	want := "[youtube] jNQXAC9IVRw: Requested format is not available. Use --list-formats for a list of available formats"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	wantFooter := want + bugReportsMessage
	err.Expected = false
	if got := err.Error(); got != wantFooter {
		t.Errorf("unexpected error: got %q, want %q", got, wantFooter)
	}
}

func TestGeoRestrictedErrorRendering(t *testing.T) {
	err := &GeoRestrictedError{Msg: "This video is not available in this country", IE: "youtube", VideoID: "abc12345678"}
	want := "[youtube] abc12345678: This video is not available in this country"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestIsExpected(t *testing.T) {
	if !IsExpected(&NoFormatsError{Expected: true}) {
		t.Error("NoFormatsError{Expected:true} should be expected")
	}
	if IsExpected(&NoFormatsError{}) {
		t.Error("NoFormatsError without Expected should not be expected")
	}
	if !IsExpected(&GeoRestrictedError{}) {
		t.Error("GeoRestrictedError should be expected")
	}
	if !IsExpected(&UnsupportedURLError{URL: "x"}) {
		t.Error("UnsupportedURLError should be expected")
	}
	if IsExpected(&ExtractorError{}) {
		t.Error("ExtractorError with Expected=false should not be expected")
	}
	if !IsExpected(&ExtractorError{Expected: true}) {
		t.Error("ExtractorError with Expected=true should be expected")
	}
}
