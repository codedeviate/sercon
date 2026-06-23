package main

import "testing"

const tagSoup = `<html><body>
  <ul>
    <li class="p"><a href="/a">Alpha<li class="p"><a href="/b">Beta
  </ul>
  <h1>Title</h1>
</body></html>`

func TestHTML_LenientParseAndCSS(t *testing.T) {
	root, err := htmlParse(tagSoup) // unclosed <li>/<a> must NOT error
	if err != nil {
		t.Fatalf("htmlParse: %v", err)
	}
	items, err := cssQueryAll(root, "li.p")
	if err != nil {
		t.Fatalf("cssQueryAll: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("li.p count = %d, want 2", len(items))
	}
	// Scoped sub-query: the <a> inside the first <li>.
	a, err := cssQueryFirst(items[0], "a")
	if err != nil || a == nil {
		t.Fatalf("cssQueryFirst a: %v node=%v", err, a)
	}
	if got, ok := nodeAttr(a, "href"); !ok || got != "/a" {
		t.Fatalf("href = %q ok=%v, want /a", got, ok)
	}
	if nodeText(a) != "Alpha" {
		t.Fatalf("text = %q, want Alpha", nodeText(a))
	}
	if nodeTag(a) != "a" {
		t.Fatalf("tag = %q, want a", nodeTag(a))
	}
}

func TestHTML_XPathAndAttributes(t *testing.T) {
	root, _ := htmlParse(`<div><a href="/x">X</a><a href="/y">Y</a></div>`)
	hrefs, err := xpathQueryAll(root, "//a/@href")
	if err != nil {
		t.Fatalf("xpathQueryAll: %v", err)
	}
	if len(hrefs) != 2 || nodeText(hrefs[0]) != "/x" || nodeText(hrefs[1]) != "/y" {
		t.Fatalf("hrefs = %v, want [/x /y]", []string{nodeText(hrefs[0]), nodeText(hrefs[1])})
	}
	if _, err := cssQueryAll(root, "<<<bad"); err == nil {
		t.Fatalf("expected error on invalid CSS selector")
	}
	if _, err := xpathQueryAll(root, "///"); err == nil {
		t.Fatalf("expected error on invalid XPath")
	}
}
