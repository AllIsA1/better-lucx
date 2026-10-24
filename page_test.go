// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func pageHTML(links []string, extra string) string {
	data, _ := json.Marshal(map[string]any{"announce": "", "emails": []string{"a"}, "links": links, "subTitle": "T" + extra})
	return "<!doctype html><html><head><title>Subscription</title>" +
		`<script data-cfasync="false" type="module" src="/x/assets/subpage.js"></script>` +
		`<script>window.X_UI_BASE_PATH="/x/";window.__SUB_PAGE_DATA__=` + string(data) + `;</script>` +
		`</head><body><div id="message"></div><div id="app"></div></body></html>`
}

func htmlResponse(body string) *SeamResponse {
	return &SeamResponse{Status: 200, Headers: map[string][]string{"Content-Type": {"text/html; charset=utf-8"}}, Body: []byte(body)}
}

func pageSeam(t *testing.T, resp *SeamResponse) (string, SeamReply) {
	t.Helper()
	c := call("subA", resp.Body, resp.Headers)
	c.Response = resp
	reply, err := subscriptionSeam(c, awgFormat{})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Action != "modify" {
		return "", reply
	}
	return string(*reply.Response.Body), reply
}

func TestTheLinksAreReadFromThePageData(t *testing.T) {
	got, ok := subPageLinks(pageHTML([]string{vless, "vpn://abc"}, ""))
	if !ok || len(got) != 2 || got[1] != "vpn://abc" {
		t.Errorf("links = %v, %v", got, ok)
	}
	// a closing tag or a semicolon inside a string does not end the data early
	tricky := pageHTML([]string{"vless://a#x</script>;y"}, "")
	if got, ok := subPageLinks(tricky); !ok || got[0] != "vless://a#x</script>;y" {
		t.Errorf("links = %v, %v", got, ok)
	}
	for name, h := range map[string]string{
		"no data":         "<html><body>hello</body></html>",
		"broken data":     "<script>window.__SUB_PAGE_DATA__={not json};</script>",
		"data not object": "<script>window.__SUB_PAGE_DATA__=[1,2];</script>",
		"no links":        `<script>window.__SUB_PAGE_DATA__={"a":1};</script>`,
		"links not list":  `<script>window.__SUB_PAGE_DATA__={"links":"x"};</script>`,
	} {
		if links, ok := subPageLinks(h); ok {
			t.Errorf("%s: links = %v", name, links)
		}
	}
}

func TestVeryShortOrEmptyPagesAreNotAProblem(t *testing.T) {
	for _, h := range []string{"", "x", "window.__SUB_PAGE_DATA", "<html>"} {
		if links, ok := subPageLinks(h); ok || links != nil {
			t.Errorf("%q: %v %v", h, links, ok)
		}
	}
	if got := injectPageBlock("", awgPageNodes([]string{nativeVPN(awgConf)})); !strings.Contains(got, pageMarker) {
		t.Error("an empty page got no block")
	}
}

func TestOnlyALineStartingWithVpnMakesANode(t *testing.T) {
	bare := base64.RawURLEncoding.EncodeToString([]byte(awgConf))
	if got := awgPageNodes([]string{bare}); len(got) != 0 {
		t.Errorf("a bare config line made a node: %+v", got)
	}
}

func TestOnlyTheOlderAmneziaWGLineBeforeTheVpnLineIsTheLegacyForm(t *testing.T) {
	vpn := nativeVPN(named("n"))
	for name, before := range map[string]string{
		"a config link": confLinkOf(named("n")),
		"another link":  vless,
		"nothing":       "",
	} {
		links := []string{before, vpn}
		if before == "" {
			links = []string{vpn}
		}
		for _, f := range awgPageNodes(links)[0].Formats {
			if f.ID == "legacy" {
				t.Errorf("%s was taken for the legacy form", name)
			}
		}
	}
	if nodes := awgPageNodes([]string{legacyLine, vpn}); len(nodes[0].Formats) != 4 {
		t.Errorf("the legacy line was not found: %+v", nodes)
	}
}

func TestTheBlockGoesBeforeAnUpperCaseBodyEndToo(t *testing.T) {
	html := strings.Replace(pageHTML(nil, ""), "</body>", "</BODY>", 1)
	out := injectPageBlock(html, awgPageNodes([]string{nativeVPN(awgConf)}))
	if i := strings.Index(out, pageMarker); i < 0 || i > strings.Index(out, "</BODY>") {
		t.Error("the block is not before the end of the body")
	}
}

func TestAPageThatHasTheBlockAlreadyIsKept(t *testing.T) {
	vpn := nativeVPN(named("again"))
	once, _ := pageSeam(t, htmlResponse(pageHTML([]string{vpn}, "")))
	if _, reply := pageSeam(t, htmlResponse(once)); reply.Action != "keep" {
		t.Error("a page with the block was changed again")
	}
}

func TestEveryNodeOfTheSubscriptionIsOfferedInItsForms(t *testing.T) {
	conf := named("node one")
	vpn := amneziaVPN(t, conf)
	nodes := awgPageNodes([]string{vless, legacyLine, vpn, "trojan://p@h:443#t"})
	if len(nodes) != 1 {
		t.Fatalf("nodes = %+v", nodes)
	}
	n := nodes[0]
	if n.Name != "node one" {
		t.Errorf("name = %q", n.Name)
	}
	ids := []string{}
	for _, f := range n.Formats {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "vpn,conf-link,legacy,conf" {
		t.Fatalf("forms = %v", ids)
	}
	want := map[string]string{"vpn": vpn, "conf-link": confLinkOf(strings.TrimRight(conf, "\n") + "\n"), "legacy": legacyLine, "conf": strings.TrimRight(conf, "\n") + "\n"}
	for _, f := range n.Formats {
		if f.Value != want[f.ID] {
			t.Errorf("%s = %q, want %q", f.ID, f.Value, want[f.ID])
		}
	}
}

func TestANodeWithoutALegacyLineHasNoLegacyForm(t *testing.T) {
	vpn := nativeVPN(named("native"))
	nodes := awgPageNodes([]string{vpn})
	if len(nodes) != 1 || len(nodes[0].Formats) != 3 || nodes[0].Formats[0].ID != "vpn" || nodes[0].Formats[1].ID != "conf-link" || nodes[0].Formats[2].ID != "conf" {
		t.Errorf("nodes = %+v", nodes)
	}
}

func TestSeveralNodesKeepTheirOwnLegacyLine(t *testing.T) {
	a, b := named("a"), strings.Replace(named("b"), "203.0.113.1", "198.51.100.7", 1)
	la, lb := "amneziawg://k@a:1?x=1#a", "amneziawg://k@b:2?x=1#b"
	nodes := awgPageNodes([]string{la, amneziaVPN(t, a), confLinkOf(a), lb, amneziaVPN(t, b)})
	if len(nodes) != 2 {
		t.Fatalf("nodes = %+v", nodes)
	}
	pick := func(n pageNode, id string) string {
		for _, f := range n.Formats {
			if f.ID == id {
				return f.Value
			}
		}
		return ""
	}
	if pick(nodes[0], "legacy") != la || pick(nodes[1], "legacy") != lb || !strings.Contains(pick(nodes[1], "conf"), "198.51.100.7") {
		t.Errorf("nodes = %+v", nodes)
	}
}

func TestAnUnnamedNodeGetsANumber(t *testing.T) {
	nodes := awgPageNodes([]string{nativeVPN(awgConf), nativeVPN(strings.Replace(awgConf, "203.0.113.1", "198.51.100.7", 1))})
	if len(nodes) != 2 || nodes[0].Name != "AmneziaWG 1" || nodes[1].Name != "AmneziaWG 2" {
		t.Errorf("names = %+v", nodes)
	}
}

func TestNoAmneziaWGLinksNoNodes(t *testing.T) {
	if got := awgPageNodes([]string{vless, "vpn://garbage", "amneziawg://k@h:1?x=1"}); len(got) != 0 {
		t.Errorf("nodes = %+v", got)
	}
}

func TestTheBlockIsPutBeforeTheEndOfTheBody(t *testing.T) {
	nodes := awgPageNodes([]string{amneziaVPN(t, named("n"))})
	out := injectPageBlock(pageHTML(nil, ""), nodes)
	i, j := strings.Index(out, pageMarker), strings.LastIndex(out, "</body>")
	if i < 0 || j < 0 || i > j || strings.Count(out, "</body>") != 1 {
		t.Fatalf("marker at %d, body end at %d", i, j)
	}
	noBody := injectPageBlock("<html><div>x</div>", nodes)
	if !strings.Contains(noBody, pageMarker) {
		t.Error("a page without </body> got no block")
	}
	if again := injectPageBlock(out, nodes); again != out {
		t.Error("a second pass changed the page")
	}
}

func TestWhatTheNodesCarryCannotBreakOutOfTheirScript(t *testing.T) {
	evil := "# </script><script>alert(1)</script> <!--   & name\n" + awgConf
	nodes := awgPageNodes([]string{nativeVPN(evil)})
	out := injectPageBlock(pageHTML(nil, ""), nodes)
	data := out[strings.Index(out, `id="`+dataID+`"`):]
	data = data[strings.Index(data, ">")+1:]
	data = data[:strings.Index(data, "</script>")]
	for _, bad := range []string{"<", ">", "&", " ", " "} {
		if strings.Contains(data, bad) {
			t.Errorf("the embedded data contains %q: %s", bad, data)
		}
	}
	var back []pageNode
	if err := json.Unmarshal([]byte(data), &back); err != nil || len(back) != 1 || !strings.Contains(back[0].Name, "<script>alert(1)</script>") {
		t.Errorf("the data does not read back: %v %+v", err, back)
	}
}

func TestTheSubscriptionPageGetsTheBlockAndNothingElseDoes(t *testing.T) {
	vpn := amneziaVPN(t, named("page node"))
	got, reply := pageSeam(t, htmlResponse(pageHTML([]string{legacyLine, vpn}, "")))
	if reply.Action != "modify" || !strings.Contains(got, pageMarker) || !strings.Contains(got, `"name":"page node"`) {
		t.Fatalf("reply = %+v", reply)
	}
	if !strings.Contains(got, `window.__SUB_PAGE_DATA__=`) || !strings.Contains(got, "/x/assets/subpage.js") {
		t.Error("the page itself was damaged")
	}
	for name, resp := range map[string]*SeamResponse{
		"no AmneziaWG":    htmlResponse(pageHTML([]string{vless}, "")),
		"not 200":         {Status: 404, Headers: map[string][]string{"Content-Type": {"text/html"}}, Body: []byte(pageHTML([]string{vpn}, ""))},
		"not html":        {Status: 200, Headers: map[string][]string{"Content-Type": {"application/json"}}, Body: []byte(pageHTML([]string{vpn}, ""))},
		"no data in page": htmlResponse("<html><body>x</body></html>"),
		"empty":           {Status: 200, Headers: map[string][]string{"Content-Type": {"text/html"}}},
	} {
		if _, reply := pageSeam(t, resp); reply.Action != "keep" {
			t.Errorf("%s: the page was changed", name)
		}
	}
	if _, reply := pageSeam(t, &SeamResponse{Status: 200, Headers: map[string][]string{"content-type": {"TEXT/HTML"}}, Body: []byte(pageHTML([]string{legacyLine, vpn}, ""))}); reply.Action != "modify" {
		t.Error("the content type is not matched ignoring case")
	}
}

func TestThePageBlockAndTheLinkModulesDoNotMix(t *testing.T) {
	r := newRig(t)
	c := call("subA", []byte(pageHTML([]string{vless}, "")), map[string][]string{"Content-Type": {"text/html"}})
	reply, err := subscriptionSeam(c, awgFormat{}, r.fix)
	if err != nil || reply.Action != "keep" {
		t.Errorf("a page was treated as a subscription: %+v %v", reply, err)
	}
	if n := r.panel.count("/inbounds/list/slim"); n != 0 {
		t.Errorf("the panel was asked %d times for a page", n)
	}
}

func TestTheEmbeddedPageScriptIsValidJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	for _, f := range []string{"page/block.js"} {
		if out, err := exec.Command(node, "--check", f).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", f, err, out)
		}
	}
	if len(pageScript) == 0 || len(pageStyle) == 0 {
		t.Error("the page assets are not embedded")
	}
}

func TestTheSeamEndpointServesThePageBlockToo(t *testing.T) {
	a := newAPIRig(t)
	vpn := amneziaVPN(t, named("through the mux"))
	c := call("subA", nil, nil)
	c.Response = htmlResponse(pageHTML([]string{legacyLine, vpn}, ""))
	raw, _ := json.Marshal(c)
	reply := postSeam(t, a, "after", raw)
	if reply.Action != "modify" || !strings.Contains(string(*reply.Response.Body), pageMarker) {
		t.Errorf("reply = %+v", reply)
	}
}

func TestTheEmbeddedScriptCannotEndItsOwnTag(t *testing.T) {
	for name, asset := range map[string]string{"script": pageScript, "style": pageStyle} {
		if strings.Contains(strings.ToLower(asset), "</script") || strings.Contains(asset, "<!--") {
			t.Errorf("the %s contains a sequence that could end its tag", name)
		}
	}
}

func TestThePageScriptsOwnTestsPass(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "--test", "page/block.test.mjs").CombinedOutput(); err != nil {
		t.Errorf("node --test page/block.test.mjs:\n%s", out)
	}
}
