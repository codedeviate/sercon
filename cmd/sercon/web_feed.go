package main

import (
	"context"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/mmcdole/gofeed"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// isoOrNil formats a parsed time as RFC3339, or returns nil when gofeed could
// not parse the date (so the field is null, never an unparsed string).
func isoOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// feedItemRaw collects format-specific extras that don't fit the normalized
// shape: enclosures, namespaced extension elements (media:*, dc:*, …), and any
// gofeed Custom entries. Best-effort: for a repeated/namespaced element only the
// first occurrence's Value (or, failing that, its Attrs) is surfaced; nested
// extension Children are not flattened. May be empty.
func feedItemRaw(it *gofeed.Item) map[string]any {
	raw := map[string]any{}
	if len(it.Enclosures) > 0 {
		encs := make([]map[string]any, len(it.Enclosures))
		for i, e := range it.Enclosures {
			encs[i] = map[string]any{"url": e.URL, "length": e.Length, "type": e.Type}
		}
		raw["enclosures"] = encs
		raw["enclosure"] = encs[0] // convenience: first enclosure
	}
	for ns, names := range it.Extensions { // map[string]map[string][]ext.Extension
		for name, exts := range names {
			if len(exts) > 0 {
				key := ns + ":" + name
				if exts[0].Value != "" {
					raw[key] = exts[0].Value
				} else if len(exts[0].Attrs) > 0 {
					raw[key] = exts[0].Attrs
				}
			}
		}
	}
	for k, v := range it.Custom {
		raw[k] = v
	}
	return raw
}

// feedAuthor returns the first author name, preferring Authors[] then the
// deprecated singular Author, else "".
func feedAuthor(it *gofeed.Item) string {
	if len(it.Authors) > 0 && it.Authors[0] != nil {
		return it.Authors[0].Name
	}
	if it.Author != nil {
		return it.Author.Name
	}
	return ""
}

// parseFeed parses RSS/Atom/JSON feed text into the normalized map model.
func parseFeed(source string) (map[string]any, error) {
	f, err := gofeed.NewParser().ParseString(source)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, len(f.Items))
	for i, it := range f.Items {
		cats := it.Categories
		if cats == nil {
			cats = []string{}
		}
		items[i] = map[string]any{
			"title":      it.Title,
			"link":       it.Link,
			"published":  isoOrNil(it.PublishedParsed),
			"updated":    isoOrNil(it.UpdatedParsed),
			"content":    it.Content,
			"summary":    it.Description,
			"author":     feedAuthor(it),
			"guid":       it.GUID,
			"categories": cats,
			"raw":        feedItemRaw(it),
		}
	}
	updated := isoOrNil(f.UpdatedParsed)
	if updated == nil {
		updated = isoOrNil(f.PublishedParsed)
	}
	return map[string]any{
		"feedType":    f.FeedType,
		"title":       f.Title,
		"description": f.Description,
		"link":        f.Link,
		"updated":     updated,
		"items":       items,
	}, nil
}

// feedParseBinding implements web.feed.parse(source) — synchronous.
func feedParseBinding(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		out, err := parseFeed(call.Argument(0).String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(out)
	}
}

// feedLoadArgs carries the on-loop-extracted arguments for web.feed.load.
type feedLoadArgs struct {
	url  string
	opts map[string]any
}

// feedLoadExtract is the on-loop extract for web.feed.load(url, opts?).
func feedLoadExtract(call goja.FunctionCall) (feedLoadArgs, error) {
	a := feedLoadArgs{url: call.Argument(0).String()}
	if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
		if m, ok := o.Export().(map[string]any); ok {
			a.opts = m
		}
	}
	return a, nil
}

// feedLoadWork is the off-loop worker for web.feed.load: fetch + parse, return
// the plain map (PromisifyAsync vm.ToValue-converts it on the loop).
func feedLoadWork(ctx context.Context, a feedLoadArgs) (map[string]any, error) {
	body, _, err := loadBytes(ctx, a.url, a.opts)
	if err != nil {
		return nil, err
	}
	return parseFeed(string(body))
}

// feedNamespace builds the web.feed sub-namespace.
func feedNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"parse": feedParseBinding(vm),
		"load":  scriptengine.PromisifyAsync(vm, loop, feedLoadExtract, feedLoadWork),
	}
}
