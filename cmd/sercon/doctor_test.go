package main

import (
	"sort"
	"testing"
)

func TestDoctorParseVersion(t *testing.T) {
	cases := map[string]string{
		"git version 2.43.0":               "2.43.0",
		"gh version 2.40.1 (2023-12-13)":   "2.40.1",
		"ChromeDriver 149.0.7827.54 (...)": "149.0.7827.54",
		"typst 0.15.0 (unknown commit)":    "0.15.0",
		"no version here":                  "",
	}
	for raw, want := range cases {
		if got := doctorParseVersion(raw); got != want {
			t.Fatalf("doctorParseVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestDoctorMajor(t *testing.T) {
	if m, ok := doctorMajor("149.0.7827.54"); !ok || m != 149 {
		t.Fatalf("major(149...) = %d,%v", m, ok)
	}
	if _, ok := doctorMajor("nope"); ok {
		t.Fatal("major(nope) should be !ok")
	}
}

func TestChromedriverConflict(t *testing.T) {
	if c, _ := chromedriverConflict("149.0.7827.54", "149.0.7827.90"); c {
		t.Fatal("same major must not conflict")
	}
	if c, d := chromedriverConflict("120.0.1.1", "149.0.1.1"); !c || d == "" {
		t.Fatalf("major mismatch must conflict (got %v %q)", c, d)
	}
	// Unparseable Chrome → no conflict asserted.
	if c, _ := chromedriverConflict("149.0.1.1", ""); c {
		t.Fatal("missing chrome version must not conflict")
	}
}

func TestDoctorRegistryShape(t *testing.T) {
	reg := doctorRegistry("darwin")
	if len(reg) == 0 {
		t.Fatal("empty registry")
	}
	seenCat := map[string]bool{}
	for _, c := range reg {
		if c.name == "" || c.category == "" || c.purpose == "" || len(c.binaries) == 0 {
			t.Fatalf("incomplete check: %#v", c)
		}
		seenCat[c.category] = true
	}
	for _, want := range []string{"git", "gh", "ai", "agentBrowser", "webdriver", "typst", "http", "clipboard", "image"} {
		if !seenCat[want] {
			t.Fatalf("registry (darwin) missing category %q", want)
		}
	}
	// Linux registry should include xclip-family clipboard, not pbcopy.
	lin := doctorRegistry("linux")
	hasPb, hasXclip := false, false
	for _, c := range lin {
		if c.name == "pbcopy" {
			hasPb = true
		}
		if c.name == "xclip" {
			hasXclip = true
		}
	}
	if hasPb || !hasXclip {
		t.Fatalf("linux registry clipboard wrong (pbcopy=%v xclip=%v)", hasPb, hasXclip)
	}
}

func TestFeatureMet(t *testing.T) {
	tools := []toolReport{
		{Name: "claude", Category: "ai", Installed: true, OK: true},
		{Name: "chromedriver", Category: "webdriver", Installed: true, OK: false}, // conflict
		{Name: "geckodriver", Category: "webdriver", Installed: true, OK: true},
		{Name: "typst", Category: "typst", Installed: false, OK: true},
	}
	// ai met (claude present+ok)
	if met, known := featureMet("ai", tools); !known || !met {
		t.Fatalf("ai: met=%v known=%v", met, known)
	}
	// webdriver met (geckodriver ok, despite chromedriver conflict)
	if met, _ := featureMet("webdriver", tools); !met {
		t.Fatal("webdriver should be met via geckodriver")
	}
	// typst not met (absent)
	if met, _ := featureMet("typst", tools); met {
		t.Fatal("typst should be unmet (absent)")
	}
	// specific binary name: chromedriver not met (conflict)
	if met, known := featureMet("chromedriver", tools); !known || met {
		t.Fatalf("chromedriver: met=%v known=%v", met, known)
	}
	// unknown requirement
	if _, known := featureMet("bogus", tools); known {
		t.Fatal("bogus must be unknown")
	}
}

func TestResolveRequires(t *testing.T) {
	tools := []toolReport{
		{Name: "git", Category: "git", Installed: true, OK: true},
		{Name: "typst", Category: "typst", Installed: false, OK: true},
	}
	unmet, err := resolveRequires([]string{"git", "typst"}, tools)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(unmet)
	if len(unmet) != 1 || unmet[0] != "typst" {
		t.Fatalf("unmet = %v, want [typst]", unmet)
	}
	if _, err := resolveRequires([]string{"git", "nope"}, tools); err == nil {
		t.Fatal("unknown requirement must error")
	}
}
