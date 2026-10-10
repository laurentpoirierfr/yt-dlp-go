package youtube

import (
	"encoding/json"
	"testing"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func mustUnmarshal(t *testing.T, s string) map[string]any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	m, _ := v.(map[string]any)
	if m == nil {
		t.Fatal("not a JSON object")
	}
	return m
}

func TestFindContinuationToken(t *testing.T) {
	cases := []string{
		// legacy continuationItemRenderer -> continuationEndpoint shape
		`{
		  "contents": {
		    "playlistVideoListRenderer": {
		      "contents": [
		        {"playlistVideoRenderer": {"videoId": "a"}},
		        {
		          "continuationItemRenderer": {
		            "continuationEndpoint": {
		              "continuationCommand": {"token": "TOK123"}
		            }
		          }
		        }
		      ]
		    }
		  }
		}`,
		// recent continuationItemViewModel shape (token + request)
		`{
		  "contents": {
		    "playlistVideoListRenderer": {
		      "contents": [
		        {"playlistVideoRenderer": {"videoId": "a"}},
		        {"continuationItemViewModel": {
		          "trigger": "CONTINUATION_TRIGGER_ON_ITEM_SHOWN",
		          "continuationCommand": {"token": "VMT", "request": "x"}
		        }}
		      ]
		    }
		  }
		}`,
		// endpoint shape actually served today
		`{
		  "contents": {
		    "playlistVideoListRenderer": {
		      "contents": [
		        {"playlistVideoRenderer": {"videoId": "a"}},
		        {"continuationItemRenderer": {"continuationEndpoint": {
		          "clickTrackingParams": "p",
		          "commandMetadata": {"webCommandMetadata": {"url": "/playlist"}},
		          "continuationCommand": {"token": "ENDPOINT", "request": "y"}
		        }}}
		      ]
		    }
		  }
		}`,
	}
	for i, json := range cases {
		if got := findContinuationToken(mustUnmarshal(t, json)); got == "" {
			t.Errorf("case %d: findContinuationToken = empty", i)
		}
	}
}

func TestPlaylistTabContinuationTokenPrefersList(t *testing.T) {
	// A stray continuation token outside the video list must be ignored.
	m := mustUnmarshal(t, `{
	  "sectionListRenderer": {
	    "contents": [
	      {"continuationItemRenderer": {"continuationEndpoint": {"continuationCommand": {"token": "WRONG"}}}},
	      {"playlistVideoListRenderer": {"contents": [{"continuationItemRenderer": {"continuationEndpoint": {"continuationCommand": {"token": "RIGHT"}}}}]}}
	    ]
	  }
	}`)
	if got := playlistTabContinuationToken(m); got != "RIGHT" {
		t.Errorf("playlistTabContinuationToken = %q, want RIGHT", got)
	}
}

func TestContinuationResponseToken(t *testing.T) {
	m := mustUnmarshal(t, `{
	  "onResponseReceivedActions": [
	    {
	      "appendContinuationItemsAction": {
	        "continuationItems": [
	          {"playlistVideoRenderer": {"videoId": "b"}},
	          {"continuationItemRenderer": {"continuationEndpoint": {"continuationCommand": {"token": "NEXT"}}}}
	        ]
	      }
	    }
	  ]
	}`)
	if got := continuationResponseToken(m); got != "NEXT" {
		t.Errorf("continuationResponseToken = %q, want NEXT", got)
	}
}

func TestCollectVideoEntries(t *testing.T) {
	m := mustUnmarshal(t, `{
	  "onResponseReceivedActions": [
	    {"appendContinuationItemsAction": {"continuationItems": [
	      {"playlistVideoRenderer": {"videoId": "aaa", "title": {"runs": [{"text": "First"}]}, "lengthSeconds": "125"}},
	      {"lockupViewModel": {
	        "contentType": "LOCKUP_CONTENT_TYPE_VIDEO",
	        "contentId": "bbb",
	        "metadata": {"lockupMetadataViewModel": {"title": {"content": "Second"}}}
	      }},
	      {"continuationItemRenderer": {"continuationEndpoint": {"continuationCommand": {"token": "T"}}}}
	    ]}},
	    {"reloadContinuationItemsCommand": {"continuationItems": [
	      {"playlistVideoRenderer": {"videoId": "ccc"}}
	    ]}}
	  ]
	}`)
	entries := collectVideoEntries(continuationItems(m))
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	want := []struct{ id, title string }{
		{"aaa", "First"}, {"bbb", "Second"}, {"ccc", ""},
	}
	for i, e := range entries {
		if e.ID != want[i].id || e.Title != want[i].title {
			t.Errorf("entry %d = %+v, want id=%q title=%q", i, e, want[i].id, want[i].title)
		}
		if e.Type != "url" || e.URL == "" {
			t.Errorf("entry %d should be a url redirect, got type=%q url=%q", i, e.Type, e.URL)
		}
	}
}

func TestCollectVideoEntriesEmpty(t *testing.T) {
	if got := collectVideoEntries(nil); got != nil {
		t.Errorf("collectVideoEntries(nil) = %v, want nil", got)
	}
	if got := collectVideoEntries(map[string]any{"x": 1}); got != nil {
		t.Errorf("collectVideoEntries(no videos) = %v, want nil", got)
	}
}

func TestPlaylistEntriesDuration(t *testing.T) {
	e := playlistEntry("aaa", "", "125")
	if e.Duration != 125 {
		t.Errorf("Duration = %v, want 125", e.Duration)
	}
	if e.ID != "aaa" || e.Title != "" {
		t.Errorf("playlistEntry fields = %+v", e)
	}
	if e.IEKey != "Youtube" {
		t.Error("playlistEntry missing extractor key")
	}
	_ = infodict.InfoDict{}
}
