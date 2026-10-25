// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// seal makes a link the way the panel does: csqtt://base64url(IV || AES-256-GCM(payload)).
func seal(t *testing.T, payload string) string {
	t.Helper()
	key := sha256.Sum256([]byte(csqttCipherSeed))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte("0123456789ab")
	return csqttPrefix + base64.RawURLEncoding.EncodeToString(append(iv, gcm.Seal(nil, iv, []byte(payload), nil)...))
}

const (
	ypUA      = "YPtun/3.7.0 (Android)"
	ypPayload = "connect?v=2&host=203.0.113.5&peer=46000&password=p%40ss%20w%2Bord&hashes=aa+bb"
)

func TestTheSealedCsqttLinkBecomesThePlainOne(t *testing.T) {
	got := csqttPlain{}.applyRaw([]string{"vless://x#a", seal(t, ypPayload)})
	want := "csqtt://connect?v=2&host=203.0.113.5&peer=46000&password=p%40ss%20w%2Bord&hashes=aa+bb"
	if got[0] != "vless://x#a" || got[1] != want {
		t.Errorf("got %q, want the second line to be %q", got, want)
	}
}

func TestPlainLinkWithoutHashesAndWithAFragmentOnTheSealedOne(t *testing.T) {
	sealed := seal(t, "connect?v=2&host=h.example&peer=9&password=pw")
	want := "csqtt://connect?v=2&host=h.example&peer=9&password=pw"
	if got := plainCsqttLink(sealed + "#Name"); got != want {
		t.Errorf("got %q, want %q (no hashes, and no #name: YPtun would read it as part of the last value)", got, want)
	}
}

func TestLinesYPtunCannotImproveAreLeftAlone(t *testing.T) {
	already := "csqtt://connect?v=2&host=h&peer=1&password=p"
	for _, l := range []string{
		already, "vless://x#a", "csqtt://", "csqtt://notbase64!!", csqttPrefix + "AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_",
		seal(t, "connect?v=2&host=&peer=1&password=p"), seal(t, "nothing"),
	} {
		if got := plainCsqttLink(l); got != l {
			t.Errorf("plainCsqttLink(%q) = %q", l, got)
		}
	}
}

func TestTheRawSubscriptionIsPlainForEveryClient(t *testing.T) {
	sealed := seal(t, ypPayload)
	raw := base64.StdEncoding.EncodeToString([]byte("vless://x#a\n" + sealed))
	for _, ua := range []string{ypUA, "Happ/1.0", ""} {
		c := call("abc", []byte(raw), map[string][]string{"Content-Type": {"text/plain"}})
		c.Request.Headers = map[string][]string{"User-Agent": {ua}}
		reply, err := subscriptionSeam(c, csqttPlain{})
		if err != nil || reply.Action != "modify" {
			t.Fatalf("%q: reply = %+v, %v", ua, reply, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(string(*reply.Response.Body))
		if err != nil || !strings.Contains(string(decoded), "csqtt://connect?v=2&host=203.0.113.5") || strings.Contains(string(decoded), sealed) {
			t.Errorf("%q: the answer = %q (%v)", ua, decoded, err)
		}
	}
}

func TestThePageOfABrowserKeepsTheSealedLink(t *testing.T) {
	sealed := seal(t, ypPayload)
	page := pageFor(`{"links":["` + sealed + `"]}`)
	reply := pageSeam(&SeamResponse{Status: 200, Body: page, Headers: map[string][]string{"Content-Type": {"text/html"}}}, "abc", nil, nil)
	if reply.Action != "keep" {
		t.Errorf("the page was changed: %+v", reply)
	}
	// the page goes through the seam with the plain-link part given too: the official CSQTT app takes the sealed link from here
	c := call("abc", page, map[string][]string{"Content-Type": {"text/html"}})
	if reply, err := subscriptionSeam(c, csqttPlain{}); err != nil || reply.Action != "keep" {
		t.Errorf("the page with the plain-link part: %+v, %v", reply, err)
	}
}

func TestThePasswordKeepsItsCharactersExactly(t *testing.T) {
	// a raw '+' in the sealed text is a plus, not a space; the unreserved characters stay as they are
	sealed := seal(t, "connect?v=2&host=h&peer=1&password=a+b~c-d_e.f")
	if got, want := plainCsqttLink(sealed), "csqtt://connect?v=2&host=h&peer=1&password=a%2Bb~c-d_e.f"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
