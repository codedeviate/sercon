package main

import (
	"testing"

	"github.com/dop251/goja"
)

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

func TestHTML_NoMatchAndAttrs(t *testing.T) {
	root, _ := htmlParse(`<div><a class="p" href="/a">A</a></div>`)

	// First-match helpers return (nil, nil) when nothing matches.
	if n, err := cssQueryFirst(root, "section"); err != nil || n != nil {
		t.Fatalf("cssQueryFirst no-match = (%v, %v), want (nil, nil)", n, err)
	}
	if n, err := xpathQueryFirst(root, "//section"); err != nil || n != nil {
		t.Fatalf("xpathQueryFirst no-match = (%v, %v), want (nil, nil)", n, err)
	}

	// Accessors are nil-safe.
	if nodeText(nil) != "" || nodeTag(nil) != "" {
		t.Fatalf("accessors not nil-safe")
	}
	if _, ok := nodeAttr(nil, "x"); ok {
		t.Fatalf("nodeAttr(nil) should report absent")
	}

	// nodeAttrs returns all attributes.
	a, _ := cssQueryFirst(root, "a")
	attrs := nodeAttrs(a)
	if attrs["class"] != "p" || attrs["href"] != "/a" || len(attrs) != 2 {
		t.Fatalf("nodeAttrs = %v, want {class:p, href:/a}", attrs)
	}
}

func TestHTMLNode_GojaHandle(t *testing.T) {
	vm := goja.New()
	root, _ := htmlParse(`<div id="root"><a class="x" href="/p">Hi</a><span>S</span></div>`)
	if err := vm.Set("doc", newHTMLNode(vm, root)); err != nil {
		t.Fatalf("set doc: %v", err)
	}
	out, err := vm.RunString(`
		const a = doc.find("a.x");
		[a.tag(), a.text(), a.attr("href"), a.attr("missing"), doc.findAll("a,span").length,
		 doc.xpath("//a/@href").text()].join("|")
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "a|Hi|/p||2|/p" {
		t.Fatalf("got %q, want a|Hi|/p||2|/p", out.String())
	}
}

func TestHTMLNode_GojaExtras(t *testing.T) {
	vm := goja.New()
	root, _ := htmlParse(`<div><a class="x" href="/p">Hi</a><span>S</span></div>`)
	if err := vm.Set("doc", newHTMLNode(vm, root)); err != nil {
		t.Fatalf("set doc: %v", err)
	}
	out, err := vm.RunString(`
		const a = doc.find("a.x");
		const missing = doc.find("section");          // no match -> null
		const firstTag = doc.findAll("a,span")[0].tag(); // chained sub-query on a findAll result
		[a.attrs().href, a.attrs().class, a.html(), missing === null, firstTag].join("|")
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// htmlquery.OutputHTML renders attributes in source order; verified against
	// the live library: <a class="x" href="/p">Hi</a>.
	const wantHTML = `<a class="x" href="/p">Hi</a>`
	want := `/p|x|` + wantHTML + `|true|a`
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
