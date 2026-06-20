package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// toolReport is one external tool's diagnostic entry (also the JS shape).
type toolReport struct {
	Name      string `json:"name"`
	Category  string `json:"category"`
	Purpose   string `json:"purpose"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
}

// doctorCheck describes a requirement to probe. binaries are PATH candidates;
// the first found wins. versionArgs runs to get a version line (default
// {"--version"}). The chromedriver entry triggers the Chrome compatibility
// check in the runner.
type doctorCheck struct {
	name        string
	category    string
	purpose     string
	binaries    []string
	versionArgs []string
}

var versionToken = regexp.MustCompile(`\d+(?:\.\d+)+`)

// doctorParseVersion extracts the first dotted-number token from a --version
// line (e.g. "git version 2.43.0" → "2.43.0"); "" when none.
func doctorParseVersion(raw string) string {
	return versionToken.FindString(strings.TrimSpace(raw))
}

// doctorMajor returns the leading integer component of a version, e.g.
// "149.0.7827.54" → 149.
func doctorMajor(v string) (int, bool) {
	v = doctorParseVersion(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.SplitN(v, ".", 2)[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

// chromedriverConflict reports whether chromedriver and Chrome major versions
// disagree. If either is unparseable/empty, no conflict is asserted.
func chromedriverConflict(cdVersion, chromeVersion string) (bool, string) {
	cd, ok1 := doctorMajor(cdVersion)
	ch, ok2 := doctorMajor(chromeVersion)
	if !ok1 || !ok2 {
		return false, ""
	}
	if cd != ch {
		return true, fmt.Sprintf("chromedriver %d vs Chrome %d (major mismatch)", cd, ch)
	}
	return false, ""
}

// doctorRegistry returns the checks appropriate to goos (clipboard/image
// backends are platform-specific).
func doctorRegistry(goos string) []doctorCheck {
	reg := []doctorCheck{
		{name: "git", category: "git", purpose: "services.git — git operations", binaries: []string{"git"}},
		{name: "gh", category: "gh", purpose: "services.gh — GitHub CLI", binaries: []string{"gh"}},
		{name: "claude", category: "ai", purpose: "services.ai — Claude provider", binaries: []string{"claude"}},
		{name: "codex", category: "ai", purpose: "services.ai — Codex provider", binaries: []string{"codex"}},
		{name: "copilot", category: "ai", purpose: "services.ai — Copilot provider", binaries: []string{"copilot"}},
		{name: "gemini", category: "ai", purpose: "services.ai — Gemini provider", binaries: []string{"gemini"}},
		{name: "agent-browser", category: "agentBrowser", purpose: "services.agentBrowser — headless Chrome automation", binaries: []string{"agent-browser"}},
		{name: "chromedriver", category: "webdriver", purpose: "services.webdriver — Chrome driver (must match Chrome major)", binaries: []string{"chromedriver"}},
		{name: "geckodriver", category: "webdriver", purpose: "services.webdriver — Firefox driver", binaries: []string{"geckodriver"}},
		{name: "typst", category: "typst", purpose: "services.typst — typesetting compiler", binaries: []string{"typst"}},
		{name: "recon", category: "http", purpose: "services.exec.http — recon backend", binaries: []string{"recon"}},
		{name: "curl", category: "http", purpose: "services.exec.http — curl backend", binaries: []string{"curl"}},
	}
	switch goos {
	case "darwin":
		reg = append(reg,
			doctorCheck{name: "pbcopy", category: "clipboard", purpose: "runtime.clipboard — write", binaries: []string{"pbcopy"}, versionArgs: []string{}},
			doctorCheck{name: "pbpaste", category: "clipboard", purpose: "runtime.clipboard — read", binaries: []string{"pbpaste"}, versionArgs: []string{}},
			doctorCheck{name: "osascript", category: "clipboard", purpose: "runtime.clipboard — image write", binaries: []string{"osascript"}},
			doctorCheck{name: "pngpaste", category: "image", purpose: "runtime.clipboard.readImage", binaries: []string{"pngpaste"}, versionArgs: []string{}},
		)
	case "linux":
		reg = append(reg,
			doctorCheck{name: "wl-copy", category: "clipboard", purpose: "runtime.clipboard — Wayland write", binaries: []string{"wl-copy"}, versionArgs: []string{}},
			doctorCheck{name: "wl-paste", category: "clipboard", purpose: "runtime.clipboard — Wayland read", binaries: []string{"wl-paste"}, versionArgs: []string{}},
			doctorCheck{name: "xclip", category: "clipboard", purpose: "runtime.clipboard — X11", binaries: []string{"xclip"}, versionArgs: []string{}},
			doctorCheck{name: "xsel", category: "clipboard", purpose: "runtime.clipboard — X11 (alt)", binaries: []string{"xsel"}, versionArgs: []string{}},
		)
	case "windows":
		reg = append(reg,
			doctorCheck{name: "clip", category: "clipboard", purpose: "runtime.clipboard — write", binaries: []string{"clip"}, versionArgs: []string{}},
			doctorCheck{name: "powershell", category: "clipboard", purpose: "runtime.clipboard — read", binaries: []string{"powershell"}},
		)
	}
	return reg
}

// featureMet reports whether a requirement (a category name OR a specific tool
// name) is satisfied: at least one matching tool is installed AND ok (no
// conflict). known is false when the name matches neither a category nor a
// tool.
func featureMet(req string, tools []toolReport) (met bool, known bool) {
	for _, t := range tools {
		if t.Category == req || t.Name == req {
			known = true
			if t.Installed && t.OK {
				return true, true
			}
		}
	}
	return false, known
}

// resolveRequires returns the requirements not met (absent or conflicted).
// An unrecognized requirement is an error (catches typos).
func resolveRequires(requires []string, tools []toolReport) ([]string, error) {
	var unmet []string
	for _, req := range requires {
		met, known := featureMet(req, tools)
		if !known {
			return nil, fmt.Errorf("services.doctor: unknown requirement %q", req)
		}
		if !met {
			unmet = append(unmet, req)
		}
	}
	return unmet, nil
}
