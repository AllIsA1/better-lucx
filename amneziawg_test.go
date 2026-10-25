// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

const awgConf = "[Interface]\nPrivateKey = peerPriv\nAddress = 10.8.0.2/32\nMTU = 1320\nJc = 8\n\n[Peer]\nPublicKey = serverPub\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = 203.0.113.1:51820\n"

func named(name string) string { return "# " + name + "\n" + awgConf }

// amneziaVPN builds the vpn:// link of an AWG inbound: Amnezia's container, compressed the way Qt does it
// (the length of the text, four bytes, then zlib), as base64url.
func amneziaVPN(t *testing.T, conf string) string {
	t.Helper()
	inner, _ := json.Marshal(map[string]any{"config": conf, "Jc": 8})
	env, _ := json.Marshal(map[string]any{
		"containers":       []any{map[string]any{"container": "amnezia-awg", "awg": map[string]any{"last_config": string(inner), "port": "51820"}}},
		"defaultContainer": "amnezia-awg",
	})
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write(env)
	_ = w.Close()
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(len(env)))
	return "vpn://" + base64.RawURLEncoding.EncodeToString(append(head, z.Bytes()...))
}

// nativeVPN is the vpn:// link of the AmneziaWG module: the config text as base64url.
func nativeVPN(conf string) string {
	return "vpn://" + base64.RawURLEncoding.EncodeToString([]byte(conf))
}

func confLinkOf(conf string) string {
	link := "amneziawg://" + base64.RawURLEncoding.EncodeToString([]byte(conf))
	if name := awgConfName(conf); name != "" {
		link += "#" + strings.ReplaceAll(strings.ReplaceAll(name, " ", "%20"), "|", "%7C")
	}
	return link
}

const legacyLine = "amneziawg://peerPriv@203.0.113.1:51820?address=10.8.0.2%2F32&jc=8#awg-user"

func awgSeam(t *testing.T, body []byte, headers map[string][]string) (string, SeamReply) {
	t.Helper()
	reply, err := subscriptionSeam(call("subA", body, headers), awgFormat{})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Action != "modify" {
		return "", reply
	}
	raw := *reply.Response.Body
	if dec, err := base64.StdEncoding.DecodeString(string(raw)); err == nil {
		return string(dec), reply
	}
	return string(raw), reply
}

func TestTheConfigLinkFollowsTheVpnLineOfAnAWGNode(t *testing.T) {
	conf := named("AWG | node 1")
	vpn := amneziaVPN(t, conf)
	got, _ := awgSeam(t, b64(vless+"\n"+legacyLine+"\n"+vpn), nil)
	want := vless + "\n" + legacyLine + "\n" + vpn + "\n" + confLinkOf(strings.TrimRight(conf, "\n")+"\n")
	if got != want {
		t.Errorf("subscription:\n%s\nwant:\n%s", got, want)
	}
}

func TestTheAmneziaWGModulesOwnLinkWorksToo(t *testing.T) {
	conf := named("awg-native")
	vpn := nativeVPN(conf)
	got, _ := awgSeam(t, b64(vpn), nil)
	if want := vpn + "\n" + confLinkOf(conf); got != want {
		t.Errorf("subscription:\n%s\nwant:\n%s", got, want)
	}
}

func TestEveryNodeGetsItsOwnLinkNextToItsOwnVpnLine(t *testing.T) {
	a, b := named("node a"), strings.Replace(named("node b"), "203.0.113.1", "198.51.100.7", 1)
	va, vb := amneziaVPN(t, a), amneziaVPN(t, b)
	got, _ := awgSeam(t, b64(legacyLine+"\n"+va+"\n"+vless+"\n"+vb+"\n"+"trojan://p@h:443#t"), nil)
	lines := strings.Split(got, "\n")
	if len(lines) != 7 || lines[1] != va || lines[3] != vless || lines[4] != vb || lines[6] != "trojan://p@h:443#t" {
		t.Fatalf("lines = %v", lines)
	}
	if !strings.HasPrefix(lines[2], "amneziawg://") || !strings.HasSuffix(lines[2], "#node%20a") || !strings.HasSuffix(lines[5], "#node%20b") {
		t.Errorf("config links: %q / %q", lines[2], lines[5])
	}
	if !bytes.Contains(decodeConfLink(t, lines[5]), []byte("198.51.100.7")) {
		t.Error("the second node got the first node's config")
	}
}

func decodeConfLink(t *testing.T, link string) []byte {
	t.Helper()
	body, _, _ := strings.Cut(strings.TrimPrefix(link, "amneziawg://"), "#")
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("not base64url: %v", err)
	}
	return raw
}

func TestAPlainTextSubscriptionIsHandledToo(t *testing.T) {
	vpn := amneziaVPN(t, named("plain"))
	got, reply := awgSeam(t, []byte(legacyLine+"\n"+vpn+"\n"), map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}})
	if reply.Action != "modify" || !strings.Contains(got, vpn+"\namneziawg://") {
		t.Errorf("subscription = %q", got)
	}
}

func TestTheConfigInTheLinkIsTheConfigInTheVpnLine(t *testing.T) {
	conf := named("same")
	for name, vpn := range map[string]string{"container": amneziaVPN(t, conf), "native": nativeVPN(conf)} {
		got, _ := awgSeam(t, b64(vpn), nil)
		lines := strings.Split(got, "\n")
		if len(lines) != 2 {
			t.Fatalf("%s: lines = %v", name, lines)
		}
		if conf2 := string(decodeConfLink(t, lines[1])); strings.TrimSpace(conf2) != strings.TrimSpace(conf) || !strings.HasSuffix(conf2, "\n") {
			t.Errorf("%s: the link carries\n%q\nnot\n%q", name, conf2, conf)
		}
	}
}

func TestRunningItAgainAddsNothing(t *testing.T) {
	vpn := amneziaVPN(t, named("again"))
	once, _ := awgSeam(t, b64(legacyLine+"\n"+vpn), nil)
	if twice, reply := awgSeam(t, b64(once), nil); reply.Action != "keep" {
		t.Errorf("a second pass changed the subscription:\n%s", twice)
	}
}

func TestALinkThePanelAlreadyAddedIsNotAddedAgain(t *testing.T) {
	conf := named("panel did it")
	vpn := amneziaVPN(t, conf)
	// the panel's own newer build writes the same line right after the vpn:// line
	body := vpn + "\n" + confLinkOf(strings.TrimRight(conf, "\n")+"\n")
	if _, reply := awgSeam(t, b64(body), nil); reply.Action != "keep" {
		t.Error("the same link was added a second time")
	}
}

func TestOnlyTheLinksOfANodeWithoutOneAreCompleted(t *testing.T) {
	a, b := named("has it"), strings.Replace(named("lacks it"), "203.0.113.1", "198.51.100.7", 1)
	va, vb := amneziaVPN(t, a), amneziaVPN(t, b)
	got, _ := awgSeam(t, b64(va+"\n"+confLinkOf(strings.TrimRight(a, "\n")+"\n")+"\n"+vb), nil)
	if n := strings.Count(got, "amneziawg://"); n != 2 {
		t.Errorf("want one link per node, got %d:\n%s", n, got)
	}
}

func TestAnINCYSubscriptionWithOnlyTheConfigLineIsLeftAlone(t *testing.T) {
	if _, reply := awgSeam(t, b64(vless+"\n"+confLinkOf(named("incy"))), nil); reply.Action != "keep" {
		t.Error("a subscription without a vpn:// line was changed")
	}
}

func TestAVpnLineThatIsNotAnAWGConfigIsLeftAlone(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), 2<<20)
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write(huge)
	_ = w.Close()
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(len(huge)))
	bomb := "vpn://" + base64.RawURLEncoding.EncodeToString(append(head, z.Bytes()...))
	noConf, _ := json.Marshal(map[string]any{"containers": []any{map[string]any{"awg": map[string]any{"last_config": `{"x":1}`}}}})
	var z2 bytes.Buffer
	w2 := zlib.NewWriter(&z2)
	_, _ = w2.Write(noConf)
	_ = w2.Close()
	for name, line := range map[string]string{
		"empty":             "vpn://",
		"not base64":        "vpn://!!!",
		"plain words":       nativeVPN("hello world"),
		"only an interface": nativeVPN("[Interface]\nPrivateKey = k\n"),
		"truncated":         "vpn://AAAA",
		"not zlib":          "vpn://" + base64.RawURLEncoding.EncodeToString([]byte("0000not zlib at all")),
		"a bomb":            bomb,
		"json without conf": "vpn://" + base64.RawURLEncoding.EncodeToString(append([]byte{0, 0, 0, 0}, z2.Bytes()...)),
		"no containers":     amneziaContainerless(t),
	} {
		if _, reply := awgSeam(t, b64(vless+"\n"+line), nil); reply.Action != "keep" {
			t.Errorf("%s: the subscription was changed", name)
		}
	}
}

func amneziaContainerless(t *testing.T) string {
	t.Helper()
	env, _ := json.Marshal(map[string]any{"containers": []any{}})
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write(env)
	_ = w.Close()
	return "vpn://" + base64.RawURLEncoding.EncodeToString(append([]byte{0, 0, 0, 0}, z.Bytes()...))
}

func TestAConfigWithoutANameGivesALinkWithoutAFragment(t *testing.T) {
	if link := awgConfLink(awgConf); strings.Contains(link, "#") {
		t.Errorf("link = %q", link)
	}
	if link := awgConfLink("# My | node 1\n" + awgConf); !strings.HasSuffix(link, "#My%20%7C%20node%201") {
		t.Errorf("link = %q", link)
	}
	if link := awgConfLink("[Interface]\nPrivateKey = k\n# Before peer\n[Peer]\nPublicKey = p\n"); !strings.HasSuffix(link, "#Before%20peer") {
		t.Errorf("link = %q", link)
	}
	if got := awgConfName("# \n[Interface]\n# Named\n[Peer]\n"); got != "Named" {
		t.Errorf("name = %q", got)
	}
}

func TestTheOtherModulesStillAddTheirLinksAfterTheConversion(t *testing.T) {
	r, c := customRig(t)
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1})}
	c.store.add("alice", []newLink{{URL: "yptun://own"}})
	vpn := amneziaVPN(t, named("mixed"))
	reply, err := subscriptionSeam(call("subA", b64(legacyLine+"\n"+vpn), nil), awgFormat{}, c)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(r.decoded(reply), "\n")
	if len(lines) != 4 || lines[1] != vpn || !strings.HasPrefix(lines[2], "amneziawg://") && strings.HasSuffix(lines[2], "#mixed") || lines[3] != "yptun://own" {
		t.Errorf("lines = %v", lines)
	}
}

func packContainer(t *testing.T, env any) string {
	t.Helper()
	raw, _ := json.Marshal(env)
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write(raw)
	_ = w.Close()
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(len(raw)))
	return "vpn://" + base64.RawURLEncoding.EncodeToString(append(head, z.Bytes()...))
}

func TestOtherShapesOfTheContainerAreRead(t *testing.T) {
	inner := func(conf string) string { b, _ := json.Marshal(map[string]any{"config": conf}); return string(b) }
	first, last := named("first container"), named("last container")
	cases := map[string]struct {
		env  any
		want string
	}{
		"the last container wins": {map[string]any{"containers": []any{
			map[string]any{"awg": map[string]any{"last_config": inner(first)}},
			map[string]any{"awg": map[string]any{"last_config": inner(last)}},
		}}, "last container"},
		"a wireguard container": {map[string]any{"containers": []any{
			map[string]any{"wireguard": map[string]any{"last_config": inner(last)}},
		}}, "last container"},
		"last_config as plain config text": {map[string]any{"containers": []any{
			map[string]any{"awg": map[string]any{"last_config": last}},
		}}, "last container"},
		"last_config as an object": {map[string]any{"containers": []any{
			map[string]any{"awg": map[string]any{"last_config": map[string]any{"config": last}}},
		}}, "last container"},
	}
	for name, c := range cases {
		got, reply := awgSeam(t, b64(packContainer(t, c.env)), nil)
		if reply.Action != "modify" || !strings.HasSuffix(strings.Split(got, "\n")[1], "#"+strings.ReplaceAll(c.want, " ", "%20")) {
			t.Errorf("%s: %q", name, got)
		}
	}
	// a container whose config text is no config is nothing
	bad := map[string]any{"containers": []any{map[string]any{"awg": map[string]any{"last_config": inner("hello")}}}}
	if _, reply := awgSeam(t, b64(packContainer(t, bad)), nil); reply.Action != "keep" {
		t.Error("a container without a config was taken")
	}
}

func TestAStreamBiggerThanAConfigCanBeIsRefused(t *testing.T) {
	big := append([]byte(awgConf), bytes.Repeat([]byte("# padding\n"), 150000)...) // a real config at the start, 1.5 MiB in all
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write(big)
	_ = w.Close()
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(len(big)))
	line := "vpn://" + base64.RawURLEncoding.EncodeToString(append(head, z.Bytes()...))
	if got := vpnConf(line); got != "" {
		t.Errorf("a payload of %d bytes was unpacked", len(got))
	}
}

func TestOnlyALineStartingWithVpnIsLookedAt(t *testing.T) {
	bare := base64.RawURLEncoding.EncodeToString([]byte(named("not a vpn line")))
	if _, reply := awgSeam(t, b64(vless+"\n"+bare), nil); reply.Action != "keep" {
		t.Error("a line without the vpn:// prefix was read as a config")
	}
}

func TestTheSameVpnLineTwiceGivesOneLink(t *testing.T) {
	vpn := amneziaVPN(t, named("twice"))
	got, _ := awgSeam(t, b64(vpn+"\n"+vless+"\n"+vpn), nil)
	if n := strings.Count(got, "amneziawg://"); n != 1 {
		t.Errorf("links = %d:\n%s", n, got)
	}
}

func TestAConfigWithOnlyAPeerIsNotAConfig(t *testing.T) {
	if _, reply := awgSeam(t, b64(nativeVPN("[Peer]\nPublicKey = p\nEndpoint = h:1\n")), nil); reply.Action != "keep" {
		t.Error("a text with only [Peer] was taken as a config")
	}
}

func TestTheSubscriptionPageOfABrowserIsLeftAlone(t *testing.T) {
	r := newRig(t)
	html := []byte("<!doctype html><html><body>vpn://abc</body></html>")
	r.keeps("sub1", html, map[string][]string{"Content-Type": {"text/html; charset=utf-8"}})
}
