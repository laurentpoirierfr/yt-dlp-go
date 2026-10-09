// Package jsc solves YouTube's JS challenges (n-parameter and signature)
// by running the EJS solver scripts under node, mirroring yt-dlp's
// yt_dlp/extractor/youtube/jsc/_builtin/{ejs,node}.py.
package jsc

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

//go:embed vendor/yt.solver.lib.min.js
var libScript string

//go:embed vendor/yt.solver.core.js
var coreScript string

// ChallengeType mirrors JsChallengeType.
type ChallengeType string

const (
	// TypeN is the n-parameter challenge.
	TypeN ChallengeType = "n"
	// TypeSig is the signature challenge.
	TypeSig ChallengeType = "sig"
)

// Request is one challenge to solve.
type Request struct {
	Type       ChallengeType
	Challenges []string
}

// Response is the result for one request.
type Response struct {
	Type    ChallengeType
	Results map[string]string // challenge -> solved value
	Err     string
}

// available reports whether node was found at init time.
var nodePath, nodeVersion = detectNode()

func detectNode() (string, string) {
	path, err := exec.LookPath("node")
	if err != nil {
		return "", ""
	}
	out, err := exec.Command(path, "--version").Output()
	ver := ""
	if err == nil {
		ver = strings.TrimSpace(string(out))
	}
	return path, ver
}

// Available reports whether a usable JS runtime was found.
func Available() bool { return nodePath != "" }

// RuntimeInfo returns the node path and version string.
func RuntimeInfo() (string, string) { return nodePath, nodeVersion }

type stdinData struct {
	Type           string     `json:"type"`
	Player         string     `json:"player"`
	Requests       []stdinReq `json:"requests"`
	OutputPreprocd bool       `json:"output_preprocessed,omitempty"`
	PreprocdPlayer string     `json:"preprocessed_player,omitempty"`
}

type stdinReq struct {
	Type       string   `json:"type"`
	Challenges []string `json:"challenges"`
}

type stdoutData struct {
	Type      string       `json:"type"`
	Error     string       `json:"error"`
	Responses []stdoutResp `json:"responses"`
}

type stdoutResp struct {
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// parseData accepts either {challenge: result} or a plain string.
func parseData(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err == nil {
		return m, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return nil, fmt.Errorf("unexpected scalar solver data %q", s)
	}
	return m, nil
}

// Solve runs the embedded solver scripts against the given player source and
// returns one Response per request, in order.
func Solve(player string, requests []Request) ([]Response, error) {
	if nodePath == "" {
		return nil, fmt.Errorf("no supported JS runtime found (node not in PATH)")
	}
	if len(requests) == 0 {
		return nil, nil
	}
	data := stdinData{
		Type:     "player",
		Player:   player,
		Requests: make([]stdinReq, 0, len(requests)),
	}
	for _, r := range requests {
		data.Requests = append(data.Requests, stdinReq{
			Type:       string(r.Type),
			Challenges: r.Challenges,
		})
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	stdin := libScript + "\nObject.assign(globalThis, lib);\n" + coreScript +
		"\nconsole.log(JSON.stringify(jsc(" + string(payload) + ")));\n"

	// Node permission flag changed from experimental to stable in v23.5.0
	args := []string{"--experimental-permission", "--no-warnings=ExperimentalWarning", "-"}
	if versionAtLeast(nodeVersion, 23, 5, 0) {
		args = []string{"--permission", "--no-warnings", "-"}
	}
	cmd := exec.Command(nodePath, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := cleanStderr(stderr.String())
		return nil, fmt.Errorf("error running node process: %v: %s", err, msg)
	}
	var out stdoutData
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		return nil, fmt.Errorf("failed to parse solver output: %w (stderr: %s)", err, cleanStderr(stderr.String()))
	}
	if out.Type == "error" {
		return nil, fmt.Errorf("solver error: %s", out.Error)
	}
	responses := make([]Response, 0, len(out.Responses))
	for i, r := range out.Responses {
		var typ ChallengeType
		if i < len(requests) {
			typ = requests[i].Type
		}
		results, perr := parseData(r.Data)
		resp := Response{Type: typ, Results: results, Err: r.Error}
		if perr != nil && resp.Err == "" {
			resp.Err = perr.Error()
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

// SolveN solves n-parameter challenges in bulk and returns challenge→result.
func SolveN(player string, challenges []string) (map[string]string, error) {
	if len(challenges) == 0 {
		return nil, nil
	}
	resps, err := Solve(player, []Request{{Type: TypeN, Challenges: challenges}})
	if err != nil {
		return nil, err
	}
	if len(resps) != 1 {
		return nil, fmt.Errorf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Err != "" {
		return nil, fmt.Errorf("n challenge failed: %s", resps[0].Err)
	}
	return resps[0].Results, nil
}

// SigSpec derives the index permutation from a solved probe string.
// probe = chr(0)..chr(len-1); solved[i] gives the source index.
func SolveSigSpec(player string, lengths []int) (map[int][]int, error) {
	probes := make([]string, 0, len(lengths))
	for _, l := range lengths {
		buf := make([]rune, l)
		for i := 0; i < l; i++ {
			buf[i] = rune(i)
		}
		probes = append(probes, string(buf))
	}
	if len(probes) == 0 {
		return nil, nil
	}
	resps, err := Solve(player, []Request{{Type: TypeSig, Challenges: probes}})
	if err != nil {
		return nil, err
	}
	if len(resps) != 1 {
		return nil, fmt.Errorf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Err != "" {
		return nil, fmt.Errorf("sig challenge failed: %s", resps[0].Err)
	}
	out := map[int][]int{}
	for probe, solved := range resps[0].Results {
		if len(solved) != len(probe) {
			return nil, fmt.Errorf("sig probe length mismatch: %d != %d", len(solved), len(probe))
		}
		spec := make([]int, len(solved))
		for i, c := range solved {
			spec[i] = int(c)
		}
		out[len(probe)] = spec
	}
	return out, nil
}

// ApplySig applies a signature index spec to an encrypted signature.
func ApplySig(sig string, spec []int) string {
	b := make([]byte, len(spec))
	for i, idx := range spec {
		if idx < 0 || idx >= len(sig) {
			return ""
		}
		b[i] = sig[idx]
	}
	return string(b)
}

var stderrNoise = []*regexp.Regexp{
	regexp.MustCompile(`^\[stdin\]:`),
	regexp.MustCompile(`^var jsc`),
	regexp.MustCompile(`^\(Use .node --trace-uncaught \.\.\.. to show where the exception was thrown\)$`),
	regexp.MustCompile(`^Node\.js v\d+\.\d+\.\d+$`),
}

func cleanStderr(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		skip := false
		for _, re := range stderrNoise {
			if re.MatchString(line) {
				skip = true
				break
			}
		}
		if !skip && strings.TrimSpace(line) != "" {
			keep = append(keep, line)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

func versionAtLeast(ver string, maj, min, patch int) bool {
	ver = strings.TrimPrefix(ver, "v")
	parts := strings.SplitN(ver, ".", 3)
	nums := make([]int, 3)
	for i, p := range parts {
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			nums[i] = nums[i]*10 + int(c-'0')
		}
	}
	want := []int{maj, min, patch}
	for i := 0; i < 3; i++ {
		if nums[i] != want[i] {
			return nums[i] > want[i]
		}
	}
	return true
}
