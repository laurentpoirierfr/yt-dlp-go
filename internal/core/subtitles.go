package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/subtitles"
)

// subtitlesRequested reports whether any subtitle-writing option is set.
func (y *YoutubeDL) subtitlesRequested() bool {
	return y.opts.WriteSubtitles || y.opts.WriteAutoSubs || y.opts.AllSubtitles
}

// convertTarget returns the --convert-subs target extension ("" = no
// conversion).
func (y *YoutubeDL) convertTarget() string {
	v := strings.ToLower(strings.TrimSpace(y.opts.ConvertSubtitles))
	if v == "" || v == "none" {
		return ""
	}
	return v
}

// writeSubtitles downloads the requested subtitle languages and writes them
// next to the media file as "<base>.<lang>.<ext>".
func (y *YoutubeDL) writeSubtitles(info *infodict.InfoDict) {
	manual := y.opts.WriteSubtitles || y.opts.AllSubtitles
	auto := y.opts.WriteAutoSubs

	spec := y.opts.SubtitlesLangs
	if y.opts.AllSubtitles {
		spec = []string{"all"}
	}
	var langs []string
	if manual {
		langs = subtitles.SelectLangs(info.Subtitles, spec)
	}
	if auto {
		for _, l := range subtitles.SelectLangs(info.AutomaticCaptions, spec) {
			if !containsString(langs, l) {
				langs = append(langs, l)
			}
		}
	}

	base := strings.TrimSuffix(info.Filepath, filepath.Ext(info.Filepath))
	target := y.convertTarget()
	if info.RequestedSubtitles == nil {
		info.RequestedSubtitles = map[string]infodict.Subtitle{}
	}
	for _, lang := range langs {
		sub, ok := y.pickSubtitle(info, manual, auto, lang)
		if !ok {
			continue
		}
		if err := y.writeSubtitle(info, base, lang, sub, target); err != nil {
			y.ReportWarning("Unable to write subtitle %s: %v", lang, err)
		}
	}
}

func (y *YoutubeDL) pickSubtitle(info *infodict.InfoDict, manual, auto bool, lang string) (infodict.Subtitle, bool) {
	if manual {
		if sub, ok := subtitles.Choose(info.Subtitles[lang], y.opts.SubtitlesFormat); ok {
			return sub, true
		}
	}
	if auto {
		return subtitles.Choose(info.AutomaticCaptions[lang], y.opts.SubtitlesFormat)
	}
	return infodict.Subtitle{}, false
}

func (y *YoutubeDL) writeSubtitle(info *infodict.InfoDict, base, lang string, sub infodict.Subtitle, target string) error {
	data, err := y.subtitleData(sub)
	if err != nil {
		return err
	}
	ext := sub.Ext
	if target != "" && !strings.EqualFold(target, ext) {
		converted, newExt, cerr := subtitles.Convert(data, ext, target)
		if cerr != nil {
			y.ReportWarning("Subtitle %s: %v", lang, cerr)
		} else {
			data, ext = converted, newExt
		}
	}
	path := base + "." + lang + "." + ext
	if err := EnsureDir(path); err != nil {
		return err
	}
	status := events.StatusStarted
	y.Sink().PostProcess(events.PostProcess{Info: info, Name: "SubtitlesConvertor", Status: status})
	if err := os.WriteFile(path, data, 0o666); err != nil {
		return err
	}
	y.ToScreen("[info] Writing video subtitles to: %s", filepath.Base(path))
	y.Sink().PostProcess(events.PostProcess{Info: info, Name: "SubtitlesConvertor", Status: events.StatusFinished})
	info.RequestedSubtitles[lang] = infodict.Subtitle{Ext: ext, URL: sub.URL, Name: sub.Name}
	return nil
}

func (y *YoutubeDL) subtitleData(sub infodict.Subtitle) ([]byte, error) {
	if sub.Data != "" {
		return []byte(sub.Data), nil
	}
	data, _, err := y.client.DownloadTo(sub.URL, sub.HTTPHeaders)
	return data, err
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
