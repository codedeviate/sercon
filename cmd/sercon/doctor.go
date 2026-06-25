package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
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
		{name: "pdftoppm", category: "pdf", purpose: "services.pdf — PDF page → image", binaries: []string{"pdftoppm"}, versionArgs: []string{"-v"}},
		{name: "pdftotext", category: "pdf", purpose: "services.pdf — PDF → text", binaries: []string{"pdftotext"}, versionArgs: []string{"-v"}},
		{name: "pdftohtml", category: "pdf", purpose: "services.pdf — PDF → HTML", binaries: []string{"pdftohtml"}, versionArgs: []string{"-v"}},
		{name: "pdfinfo", category: "pdf", purpose: "services.pdf — PDF metadata", binaries: []string{"pdfinfo"}, versionArgs: []string{"-v"}},
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

const doctorCheckTimeout = 3 * time.Second

// runVersion runs `bin args...` and returns the parsed version from combined
// output. With no args (versionArgs == []) it skips execution (tools like
// pbcopy have no --version) and returns "".
func runVersion(ctx context.Context, bin string, args []string) string {
	if args == nil {
		args = []string{"--version"}
	}
	if len(args) == 0 {
		return ""
	}
	runCtx, cancel := context.WithTimeout(ctx, doctorCheckTimeout)
	defer cancel()
	out, err := exec.CommandContext(runCtx, bin, args...).CombinedOutput() //nolint:gosec // fixed tool + version flag
	if err != nil && len(out) == 0 {
		return ""
	}
	line := out
	if i := strings.IndexByte(string(out), '\n'); i >= 0 {
		line = out[:i]
	}
	return doctorParseVersion(string(line))
}

// detectChromeVersion finds an installed Chrome/Chromium and returns its
// version string ("" if not found).
func detectChromeVersion(ctx context.Context) string {
	candidates := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium")
	}
	for _, c := range candidates {
		bin := c
		if !strings.Contains(c, "/") {
			p, err := exec.LookPath(c)
			if err != nil {
				continue
			}
			bin = p
		} else if _, err := os.Stat(bin); err != nil {
			continue
		}
		if v := runVersion(ctx, bin, []string{"--version"}); v != "" {
			return v
		}
	}
	return ""
}

// runDoctor probes every registry check concurrently and returns the full
// report plus whether any compatibility conflict was found.
func runDoctor(ctx context.Context) (tools []toolReport, anyConflict bool) {
	reg := doctorRegistry(runtime.GOOS)
	reports := make([]toolReport, len(reg))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // small worker pool
	for i, c := range reg {
		wg.Add(1)
		go func(i int, c doctorCheck) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := toolReport{Name: c.name, Category: c.category, Purpose: c.purpose, OK: true}
			path := ""
			for _, b := range c.binaries {
				if p, err := exec.LookPath(b); err == nil {
					path = p
					break
				}
			}
			if path == "" {
				reports[i] = r // not installed; ok stays true
				return
			}
			r.Installed = true
			r.Version = runVersion(ctx, path, c.versionArgs)
			reports[i] = r
		}(i, c)
	}
	wg.Wait()

	// chromedriver↔Chrome compatibility.
	for i := range reports {
		if reports[i].Name == "chromedriver" && reports[i].Installed {
			chrome := detectChromeVersion(ctx)
			if chrome == "" {
				reports[i].Detail = "Chrome not found; cannot verify version match"
				break
			}
			if conflict, detail := chromedriverConflict(reports[i].Version, chrome); conflict {
				reports[i].OK = false
				reports[i].Detail = detail
				anyConflict = true
			} else {
				reports[i].Detail = fmt.Sprintf("matches Chrome %s", chrome)
			}
			break
		}
	}
	return reports, anyConflict
}

// doctorOp backs services.doctor(requires?). Returns { ok, satisfied, unmet,
// tools }. requires entries are feature/category names or specific tool names;
// an unknown name throws.
func doctorOp(ctx context.Context, call goja.FunctionCall) (any, error) {
	var requires []string
	if arr, ok := call.Argument(0).Export().([]any); ok {
		for _, e := range arr {
			if s, ok := e.(string); ok {
				requires = append(requires, s)
			}
		}
	}
	tools, anyConflict := runDoctor(ctx)
	unmet, err := resolveRequires(requires, tools)
	if err != nil {
		return nil, err
	}
	if unmet == nil {
		unmet = []string{}
	}
	toolVals := make([]any, len(tools))
	for i, t := range tools {
		o := scriptengine.NewOrdered().
			Set("name", t.Name).Set("category", t.Category).Set("purpose", t.Purpose).
			Set("installed", t.Installed).Set("version", versionOrNull(t.Version)).
			Set("ok", t.OK)
		if t.Detail != "" {
			o.Set("detail", t.Detail)
		}
		toolVals[i] = o
	}
	unmetVals := make([]any, len(unmet))
	for i, u := range unmet {
		unmetVals[i] = u
	}
	return scriptengine.NewOrdered().
		Set("ok", !anyConflict).
		Set("satisfied", len(unmet) == 0).
		Set("unmet", unmetVals).
		Set("tools", toolVals), nil
}

// versionOrNull returns the version string or nil (→ JS null) when empty.
func versionOrNull(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// writeDoctor prints the report grouped by category. anyConflict drives the
// trailing summary line (the caller maps a conflict to a non-zero exit).
func writeDoctor(w io.Writer, tools []toolReport, anyConflict bool) {
	// group by category, preserving registry order of first appearance
	order := []string{}
	seen := map[string]bool{}
	for _, t := range tools {
		if !seen[t.Category] {
			seen[t.Category] = true
			order = append(order, t.Category)
		}
	}
	fmt.Fprintln(w, "sercon doctor — external requirements")
	fmt.Fprintln(w, "  ✓ installed   ⚠ conflict   – not installed (optional)")
	fmt.Fprintln(w, "")
	for _, cat := range order {
		fmt.Fprintf(w, "%s:\n", cat)
		for _, t := range tools {
			if t.Category != cat {
				continue
			}
			glyph := "–"
			if t.Installed && t.OK {
				glyph = "✓"
			} else if t.Installed && !t.OK {
				glyph = "⚠"
			}
			ver := t.Version
			if ver == "" {
				ver = "—"
			}
			line := fmt.Sprintf("  %s %-14s %-16s %s", glyph, t.Name, ver, t.Purpose)
			if t.Detail != "" {
				line += "  [" + t.Detail + "]"
			}
			fmt.Fprintln(w, line)
		}
	}
	fmt.Fprintln(w, "")
	if anyConflict {
		fmt.Fprintln(w, "⚠ compatibility conflict detected (exit 5).")
	} else {
		fmt.Fprintln(w, "No conflicts. (Missing tools are optional — install only what you use.)")
	}
}
