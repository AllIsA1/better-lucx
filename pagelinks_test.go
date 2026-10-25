// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type addLines []string

func (a addLines) apply(_ string, lines []string) []string { return appendMissing(lines, a) }

const pageHead = `<!doctype html><html><head><title>sub</title><script>window.X_UI_BASE_PATH="/";window.__SUB_PAGE_DATA__=`
const pageTail = `;</script></head><body><div id="root"></div></body></html>`

func pageFor(data string) []byte { return []byte(pageHead + data + pageTail) }

func embedded(t *testing.T, page []byte) map[string]any {
	t.Helper()
	s := string(page)
	i := strings.Index(s, pageDataMarker)
	if i < 0 {
		t.Fatal("no page data")
	}
	var data map[string]any
	dec := json.NewDecoder(strings.NewReader(s[i+len(pageDataMarker):]))
	if err := dec.Decode(&data); err != nil {
		t.Fatalf("the page data is not JSON any more: %v", err)
	}
	return data
}

func TestThePageListGetsTheModulesLines(t *testing.T) {
	in := pageFor(`{"sId":"abc","links":["vless://x#a"],"announce":"Привет <b>","subUrl":"https://h/sub/abc","total":"1 GB"}`)
	out, changed := pageWithLinks(in, "abc", []subModule{addLines{"openflux://v1/AAAA"}}, nil)
	if !changed {
		t.Fatal("unchanged")
	}
	data := embedded(t, out)
	links, _ := data["links"].([]any)
	if len(links) != 2 || links[0] != "vless://x#a" || links[1] != "openflux://v1/AAAA" {
		t.Errorf("links = %v", links)
	}
	if data["announce"] != "Привет <b>" || data["subUrl"] != "https://h/sub/abc" || data["total"] != "1 GB" || data["sId"] != "abc" {
		t.Errorf("another field was damaged: %v", data)
	}
	s := string(out)
	if !strings.HasPrefix(s, pageHead) || !strings.HasSuffix(s, pageTail) {
		t.Errorf("the page around the data changed:\n%s", s)
	}
	if strings.Contains(s[strings.Index(s, pageDataMarker):strings.Index(s, "</script>")], "<b>") {
		t.Error("a < or > in the data must stay escaped inside a script tag")
	}
}

func TestThePageIsLeftAloneWhenNothingChanges(t *testing.T) {
	in := pageFor(`{"links":["openflux://v1/AAAA"]}`)
	if out, changed := pageWithLinks(in, "abc", []subModule{addLines{"openflux://v1/AAAA"}}, nil); changed || string(out) != string(in) {
		t.Error("a line that is there already was added again")
	}
	if _, changed := pageWithLinks(in, "abc", nil, nil); changed {
		t.Error("no modules, no change")
	}
}

func TestThePageIsLeftAloneWhenItIsNotTheKnownShape(t *testing.T) {
	for name, page := range map[string][]byte{
		"a custom theme without the data": []byte("<html>no data here</html>"),
		"data that is not json":           pageFor(`{not json`),
		"no links":                        pageFor(`{"sId":"abc"}`),
		"links of another type":           pageFor(`{"links":{"a":1}}`),
	} {
		if out, changed := pageWithLinks(page, "abc", []subModule{addLines{"openflux://v1/AAAA"}}, nil); changed || string(out) != string(page) {
			t.Errorf("%s: the page was changed", name)
		}
	}
}

func TestTheSeamTakesThePageOfABrowserAndNotTheRawSubscriptionPath(t *testing.T) {
	call := call("abc", pageFor(`{"links":["vless://x#a"]}`), map[string][]string{"Content-Type": {"text/html; charset=utf-8"}})
	reply, err := subscriptionSeam(call, addLines{"openflux://v1/AAAA"})
	if err != nil || reply.Action != "modify" || reply.Response == nil || reply.Response.Body == nil {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	if links := embedded(t, *reply.Response.Body)["links"].([]any); len(links) != 2 {
		t.Errorf("links = %v", links)
	}
	// a plain-text answer goes the raw way: lines in, lines out, no page around them
	raw := call
	raw.Response = &SeamResponse{Status: 200, Body: []byte("vless://x#a\nvless://y#b"), Headers: map[string][]string{"Content-Type": {"text/plain"}}}
	reply, err = subscriptionSeam(raw, addLines{"openflux://v1/AAAA"})
	if err != nil || reply.Action != "modify" || !strings.HasSuffix(string(*reply.Response.Body), "openflux://v1/AAAA") || strings.Contains(string(*reply.Response.Body), "<") {
		t.Errorf("reply = %+v, %v", reply, err)
	}
}
