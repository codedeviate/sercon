package main

import "testing"

const rssFixture = `<?xml version="1.0"?>
<rss version="2.0" xmlns:media="http://search.yahoo.com/mrss/">
  <channel>
    <title>My Blog</title><link>https://blog.example</link><description>desc</description>
    <item>
      <title>First</title><link>https://blog.example/1</link>
      <pubDate>Mon, 02 Jan 2006 15:04:05 +0000</pubDate>
      <guid>g1</guid><category>go</category>
      <enclosure url="https://blog.example/a.mp3" length="123" type="audio/mpeg"/>
      <media:content url="https://blog.example/img.png"/>
    </item>
  </channel>
</rss>`

const atomFixture = `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Blog</title><link href="https://a.example"/>
  <updated>2006-01-02T15:04:05Z</updated>
  <entry><title>AE</title><link href="https://a.example/e"/>
    <updated>2006-01-02T15:04:05Z</updated><id>ae1</id>
    <summary>sum</summary></entry>
</feed>`

func TestFeed_NormalizeRSSAndAtom(t *testing.T) {
	rss, err := parseFeed(rssFixture)
	if err != nil {
		t.Fatalf("parseFeed rss: %v", err)
	}
	if rss["feedType"] != "rss" || rss["title"] != "My Blog" {
		t.Fatalf("rss header wrong: %v", rss)
	}
	if rss["description"] != "desc" || rss["link"] != "https://blog.example" {
		t.Fatalf("rss top-level fields wrong: desc=%v link=%v", rss["description"], rss["link"])
	}
	items := rss["items"].([]map[string]any)
	if len(items) != 1 || items[0]["title"] != "First" || items[0]["link"] != "https://blog.example/1" {
		t.Fatalf("rss item wrong: %v", items)
	}
	if items[0]["published"] == nil {
		t.Fatalf("expected normalized published date")
	}
	raw := items[0]["raw"].(map[string]any)
	if _, ok := raw["enclosure"]; !ok {
		t.Fatalf("expected enclosure in raw, got %v", raw)
	}
	media, ok := raw["media:content"].(map[string]string)
	if !ok || media["url"] != "https://blog.example/img.png" {
		t.Fatalf("expected media:content extension in raw, got %v", raw["media:content"])
	}

	atom, err := parseFeed(atomFixture)
	if err != nil {
		t.Fatalf("parseFeed atom: %v", err)
	}
	if atom["feedType"] != "atom" {
		t.Fatalf("feedType = %v, want atom", atom["feedType"])
	}
	aitems := atom["items"].([]map[string]any)
	if aitems[0]["summary"] != "sum" || aitems[0]["updated"] == nil {
		t.Fatalf("atom item normalization wrong: %v", aitems[0])
	}
	if cats, ok := aitems[0]["categories"].([]string); !ok || cats == nil || len(cats) != 0 {
		t.Fatalf("atom categories should be empty non-nil slice, got %#v", aitems[0]["categories"])
	}
}

func TestFeed_MalformedThrows(t *testing.T) {
	if _, err := parseFeed("not a feed at all"); err == nil {
		t.Fatalf("expected error on non-feed input")
	}
}
