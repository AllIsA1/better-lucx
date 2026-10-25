// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// Plain CSQTT links in the subscription. The panel (lucx.310 on) gives a CSQTT inbound as a sealed link,
// csqtt://<base64url>. The official CSQTT app reads only that form, but it never fetches a subscription: a person pastes
// the link into it, and the page of a browser (which keeps the sealed link) is where it comes from. The proxy clients
// that do fetch the subscription (YPtun among them) read only the plain form,
// csqtt://connect?v=2&host=..&peer=..&password=..[&hashes=a+b], and skip a sealed link. So the raw subscription gets the
// plain form of every sealed CSQTT line, for any client.

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
)

// csqttCipherSeed is the fixed seed the panel seals the CSQTT links with (upstream v2.5.0, web_panel.rs).
const csqttCipherSeed = "csqtt-chiper-key"

// rawOnly is a part of the plugin that changes the raw subscription a proxy client fetches and leaves the page of a
// browser as the panel made it.
type rawOnly interface {
	applyRaw(lines []string) []string
}

// csqttPlain gives the proxy clients the plain CSQTT links.
type csqttPlain struct{}

func (csqttPlain) applyRaw(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = plainCsqttLink(l)
	}
	return out
}

// plainCsqttLink is the plain form of a sealed CSQTT link; any other line (a plain CSQTT link included) is
// returned as it is.
func plainCsqttLink(line string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), csqttPrefix)
	if !ok || rest == "" {
		return line
	}
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		rest = rest[:i] // a #name is no part of the sealed text
	}
	payload, err := openSealedLink(rest)
	if err != nil {
		return line
	}
	_, query, ok := strings.Cut(payload, "?")
	if !ok {
		return line
	}
	q, err := url.ParseQuery(strings.ReplaceAll(query, "+", "%2B")) // '+' separates the hashes: it is not a space
	if err != nil {
		return line
	}
	host, peer, password := q.Get("host"), q.Get("peer"), q.Get("password")
	if host == "" || peer == "" || password == "" {
		return line
	}
	out := csqttPrefix + "connect?v=2&host=" + escapeUnreserved(host) + "&peer=" + escapeUnreserved(peer) +
		"&password=" + escapeUnreserved(password)
	if hashes := splitHashes(q.Get("hashes")); len(hashes) > 0 {
		for i, h := range hashes {
			hashes[i] = escapeUnreserved(h)
		}
		out += "&hashes=" + strings.Join(hashes, "+")
	}
	return out
}

func splitHashes(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return strings.ContainsRune(",;+ \t\r\n", r) })
}

// escapeUnreserved percent-encodes everything but the RFC 3986 unreserved characters, as YPtun itself does.
func escapeUnreserved(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-_.~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// openSealedLink opens base64url(IV || AES-256-GCM(payload)), the key being SHA-256 of the seed.
func openSealedLink(b64 string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(b64, "="))
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(csqttCipherSeed))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize()+gcm.Overhead() {
		return "", errors.New("too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
