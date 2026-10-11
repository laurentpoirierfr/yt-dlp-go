package subtitles

import (
	"encoding/json"
	"fmt"
	"strings"
)

// VTTToSRT converts a WebVTT document to SubRip (SRT).
func VTTToSRT(vtt string) string {
	lines := strings.Split(strings.ReplaceAll(vtt, "\r\n", "\n"), "\n")
	var cues []string
	i := 0
	idx := 0
	for i < len(lines) {
		line := lines[i]
		if !strings.Contains(line, "-->") {
			i++
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			i++
			continue
		}
		start, end := normVTTTime(fields[0]), normVTTTime(fields[2])
		i++
		var text []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			text = append(text, stripTags(lines[i]))
			i++
		}
		idx++
		cues = append(cues, fmt.Sprintf("%d\n%s --> %s\n%s\n\n", idx, start, end, strings.Join(text, "\n")))
	}
	return strings.Join(cues, "")
}

func normVTTTime(s string) string {
	s = strings.ReplaceAll(s, ".", ",")
	if strings.Count(s, ":") == 1 {
		s = "00:" + s
	}
	return s
}

func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

type json3Doc struct {
	Events []struct {
		TStartMs    int64 `json:"tStartMs"`
		DDurationMs int64 `json:"dDurationMs"`
		Segs        []struct {
			UTF8 string `json:"utf8"`
		} `json:"segs"`
	} `json:"events"`
}

func parseJSON3(data []byte) ([]cueData, error) {
	var doc json3Doc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var cues []cueData
	for i, ev := range doc.Events {
		if len(ev.Segs) == 0 {
			continue
		}
		var sb strings.Builder
		for _, s := range ev.Segs {
			sb.WriteString(s.UTF8)
		}
		text := strings.TrimRight(sb.String(), "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		end := ev.TStartMs + ev.DDurationMs
		if end <= ev.TStartMs && i+1 < len(doc.Events) {
			end = doc.Events[i+1].TStartMs
		}
		cues = append(cues, cueData{startMs: ev.TStartMs, endMs: end, text: text})
	}
	return cues, nil
}

type cueData struct {
	startMs int64
	endMs   int64
	text    string
}

func msTime(ms int64, comma bool) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	milli := ms % 1000
	sep := "."
	if comma {
		sep = ","
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", h, m, s, sep, milli)
}

// JSON3ToSRT converts a YouTube json3 caption document to SubRip (SRT).
func JSON3ToSRT(data []byte) string {
	cues, err := parseJSON3(data)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, msTime(c.startMs, true), msTime(c.endMs, true), c.text)
	}
	return b.String()
}

// JSON3ToVTT converts a YouTube json3 caption document to WebVTT.
func JSON3ToVTT(data []byte) string {
	cues, err := parseJSON3(data)
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, c := range cues {
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", msTime(c.startMs, false), msTime(c.endMs, false), c.text)
	}
	return b.String()
}
