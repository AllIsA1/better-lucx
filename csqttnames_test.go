// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func csqttInbound(port int, remark string) map[string]any {
	return map[string]any{"id": port, "protocol": "csqtt", "remark": remark, "port": port, "enable": true}
}

func csqttLine(peer string) string {
	return "csqtt://connect?host=203.0.113.5&password=pw&peer=" + peer + "&v=2&hashes=aa+bb"
}

// The panel from lucx.310 on gives the line as csqtt://<base64url sealed payload>: no query to read the port from.
const sealedLine = "csqtt://AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123"

func namerRig(t *testing.T, inbounds ...map[string]any) *csqttNames {
	t.Helper()
	r := newRig(t)
	r.panel.inbounds = append(r.panel.inbounds, inbounds...)
	n := newCsqttNames(r.panel)
	n.cm.firstLoadWait = 2 * 1e9
	return n
}

func TestTheNameOfACsqttLineIsItsInboundsRemark(t *testing.T) {
	n := namerRig(t, csqttInbound(46000, "Польша CSQTT"))
	if got := n.nameOf(sealedLine); got != "Польша CSQTT" {
		t.Errorf("sealed line: %q", got)
	}
	if got := n.nameOf(csqttLine("46000")); got != "Польша CSQTT" {
		t.Errorf("plain line: %q", got)
	}
	for _, other := range []string{"vless://x#a", "openflux://v1/AAAA", sealedLine + "#Mine"} {
		if got := n.nameOf(other); got != "" {
			t.Errorf("nameOf(%q) = %q, only a CSQTT line with no name of its own is named", other, got)
		}
	}
}

func TestCsqttNameFollowsThePeerPortAmongSeveralInbounds(t *testing.T) {
	rows := []string{"46000\tOne", "46010\tTwo"}
	if got := csqttName(csqttLine("46010"), rows); got != "Two" {
		t.Errorf("name = %q, want Two", got)
	}
	if got := csqttName(csqttLine("9"), rows); got != "" {
		t.Errorf("name = %q, a line that fits none of several inbounds stays unnamed", got)
	}
	if got := csqttName(sealedLine, rows); got != "" {
		t.Errorf("name = %q: a sealed line cannot be told apart among several inbounds", got)
	}
	if got := csqttName(sealedLine, rows[:1]); got != "One" {
		t.Errorf("name = %q, the only inbound names a line whatever its port", got)
	}
}

// The raw subscription must not carry the name: the CSQTT app reads the whole line after csqtt:// as the
// sealed payload and drops a link with a #name in it.
func TestTheRawSubscriptionKeepsTheCsqttLineAsItIs(t *testing.T) {
	n := namerRig(t, csqttInbound(46000, "Польша CSQTT"))
	raw := base64.StdEncoding.EncodeToString([]byte("vless://x#a\n" + sealedLine))
	reply, err := subscriptionSeam(call("abc", []byte(raw), map[string][]string{"Content-Type": {"text/plain"}}), n)
	if err != nil || reply.Action != "keep" {
		t.Errorf("reply = %+v, %v: the line was changed", reply, err)
	}
}

func TestThePageGetsLinkNamesForCsqttAndOpenFluxAndLeavesTheLinksAlone(t *testing.T) {
	n := namerRig(t, csqttInbound(46000, "Польша CSQTT"))
	f := openfluxNamerRig(t, "openflux://v1/AAAA", "Польша OpenFlux")
	if f.nameOf("openflux://v1/AAAA") != "Польша OpenFlux" || f.nameOf("vless://x") != "" {
		t.Fatal("the OpenFlux namer does not name its links")
	}
	var onlyNames []namer = []namer{f, n}
	links := []string{"vless://x#a", sealedLine, "openflux://v1/AAAA", "openflux://v1/OTHER"}
	data, _ := json.Marshal(map[string]any{"links": links})
	page := []byte(`<html><head><script>window.__SUB_PAGE_DATA__=` + string(data) + `;</script></head></html>`)
	body, changed := pageWithLinks(page, "abc", nil, onlyNames)
	if !changed {
		t.Fatal("the page was not changed")
	}
	got := embedded(t, body)
	var gotLinks, names []string
	raw, _ := json.Marshal(got["links"])
	_ = json.Unmarshal(raw, &gotLinks)
	raw, _ = json.Marshal(got["linkNames"])
	_ = json.Unmarshal(raw, &names)
	if strings.Join(gotLinks, "|") != strings.Join(links, "|") {
		t.Errorf("the links changed: %v", gotLinks)
	}
	want := []string{"", "Польша CSQTT", "Польша OpenFlux", ""}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Errorf("linkNames = %q, want %q: one per link, empty where there is nothing to say", names, want)
	}
}

func TestAPageWithNothingToNameIsLeftAlone(t *testing.T) {
	n := namerRig(t)
	page := pageFor(`{"links":["vless://x#a"]}`)
	if out, changed := pageWithLinks(page, "abc", nil, []namer{n}); changed || string(out) != string(page) {
		t.Error("a page with nothing to name was changed")
	}
}
