// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "AmneziaWG link format" module: next to every vpn:// line of a subscription, the same connection as
// amneziawg://<base64url .conf>#name. That is the form INCY imports; the panel gives it to INCY alone.

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"strings"
)

// maxConfBytes bounds what is unpacked from a vpn:// line: a config is a few hundred bytes.
const maxConfBytes = 1 << 20

// awgFormat adds the config link after each vpn:// line that carries an AmneziaWG config.
type awgFormat struct{}

func (awgFormat) apply(_ string, lines []string) []string {
	have := make(map[string]bool, len(lines))
	for _, l := range lines {
		have[strings.TrimSpace(l)] = true
	}
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		out = append(out, line)
		if !strings.HasPrefix(strings.TrimSpace(line), "vpn://") {
			continue
		}
		conf := vpnConf(line)
		if conf == "" {
			continue
		}
		if link := awgConfLink(conf); !have[link] {
			have[link] = true
			out = append(out, link)
		}
	}
	return out
}

// awgConfLink is amneziawg://<base64url conf>#<name>, the name being the "# name" comment of the config.
func awgConfLink(conf string) string {
	link := "amneziawg://" + base64.RawURLEncoding.EncodeToString([]byte(conf))
	if name := awgConfName(conf); name != "" {
		link += "#" + strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
	}
	return link
}

// awgConfName is the first non-empty "# comment" line of a config: the remark the panel stamps on top of
// the AWG links' configs, or the one the AmneziaWG module puts before [Peer].
func awgConfName(conf string) string {
	for _, line := range strings.Split(conf, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#"); ok {
			if name := strings.TrimSpace(rest); name != "" {
				return name
			}
		}
	}
	return ""
}

// vpnConf reads the awg-quick config out of a vpn:// line, "" when the line holds none. Two forms exist:
// the AmneziaWG module writes the config text as base64url; the AWG links wrap it in Amnezia's container,
// a JSON document (containers[].awg.last_config, itself JSON with the text under "config") that Qt packs as
// its length (four bytes, big endian) followed by zlib data.
func vpnConf(line string) string {
	body := strings.TrimPrefix(strings.TrimSpace(line), "vpn://")
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(body, "="))
	if err != nil {
		return ""
	}
	if isConf(string(raw)) {
		return string(raw)
	}
	if len(raw) <= 4 {
		return ""
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return ""
	}
	defer zr.Close()
	payload, err := io.ReadAll(io.LimitReader(zr, maxConfBytes+1))
	if err != nil || len(payload) > maxConfBytes {
		return ""
	}
	conf := confFromPayload(payload)
	if conf == "" {
		return ""
	}
	return strings.TrimRight(conf, "\n") + "\n"
}

func isConf(text string) bool {
	return strings.Contains(text, "[Interface]") && strings.Contains(text, "[Peer]")
}

// confFromPayload takes the config out of an unpacked payload: the bare text, or the container.
func confFromPayload(payload []byte) string {
	if text := strings.TrimSpace(string(payload)); text != "" && text[0] != '{' {
		if isConf(text) {
			return text
		}
		return ""
	}
	var env struct {
		Containers []map[string]json.RawMessage `json:"containers"`
	}
	if json.Unmarshal(payload, &env) != nil || len(env.Containers) == 0 {
		return ""
	}
	last := env.Containers[len(env.Containers)-1]
	for _, key := range []string{"awg", "wireguard"} {
		var proto struct {
			LastConfig json.RawMessage `json:"last_config"`
		}
		if raw, ok := last[key]; ok && json.Unmarshal(raw, &proto) == nil {
			if conf := confFromLastConfig(proto.LastConfig); conf != "" {
				return conf
			}
		}
	}
	return ""
}

// confFromLastConfig: last_config is a JSON string holding {"config": text}, or such an object itself.
func confFromLastConfig(raw json.RawMessage) string {
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		var inner struct {
			Config string `json:"config"`
		}
		if json.Unmarshal([]byte(asString), &inner) == nil && strings.Contains(inner.Config, "[Interface]") {
			return strings.TrimSpace(inner.Config)
		}
		if strings.Contains(asString, "[Interface]") {
			return strings.TrimSpace(asString)
		}
		return ""
	}
	var asObject struct {
		Config string `json:"config"`
	}
	if json.Unmarshal(raw, &asObject) == nil && strings.Contains(asObject.Config, "[Interface]") {
		return strings.TrimSpace(asObject.Config)
	}
	return ""
}
