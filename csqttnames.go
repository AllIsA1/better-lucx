// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "CSQTT name" module: the csqtt:// line of a subscription has no #name, so a proxy client calls the
// profile "csqtt [host]". This module adds the inbound's remark as the name.

package main

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const csqttPrefix = "csqtt://"

// csqttNames knows the CSQTT inbounds (their ports and remarks) and names the lines that lack a name.
type csqttNames struct {
	*cachedMap
	api panelAPI
}

// csqttKey is where the module keeps its inbounds in the cached map: "port\tremark" per inbound, not per
// subscription (the line carries no client).
const csqttKey = "csqtt"

func newCsqttNames(api panelAPI) *csqttNames {
	n := &csqttNames{api: api}
	n.cachedMap = newCachedMap("csqtt", n.load)
	return n
}

func (n *csqttNames) load(ctx context.Context) (map[string][]string, error) {
	var inbounds []struct {
		Protocol string `json:"protocol"`
		Remark   string `json:"remark"`
		Port     int    `json:"port"`
	}
	if err := n.api.Do(ctx, http.MethodGet, "/inbounds/list/slim", nil, &inbounds); err != nil {
		return nil, err
	}
	var rows []string
	for _, ib := range inbounds {
		if ib.Protocol == "csqtt" && strings.TrimSpace(ib.Remark) != "" {
			rows = append(rows, strconv.Itoa(ib.Port)+"\t"+strings.TrimSpace(ib.Remark))
		}
	}
	return map[string][]string{csqttKey: rows}, nil
}

func (n *csqttNames) apply(_ string, lines []string) []string {
	var rows []string
	for _, l := range lines {
		if isUnnamedCsqtt(l) {
			rows = n.linksFor(csqttKey)
			break
		}
	}
	if len(rows) == 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l
		if !isUnnamedCsqtt(l) {
			continue
		}
		if name := csqttName(l, rows); name != "" {
			out[i] = strings.TrimSpace(l) + "#" + url.PathEscape(name)
		}
	}
	return out
}

func isUnnamedCsqtt(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, csqttPrefix) && !strings.Contains(line, "#")
}

// csqttName picks the remark of the inbound a line belongs to: the one whose port is the line's peer port, or
// the only CSQTT inbound there is (the panel allows one per host, and its listen address may differ from the
// inbound's port).
func csqttName(line string, rows []string) string {
	peer := ""
	if i := strings.IndexByte(line, '?'); i >= 0 {
		if q, err := url.ParseQuery(strings.TrimSpace(line)[i+1:]); err == nil {
			peer = q.Get("peer")
		}
	}
	for _, row := range rows {
		port, remark, _ := strings.Cut(row, "\t")
		if peer != "" && port == peer {
			return remark
		}
	}
	if len(rows) == 1 {
		_, remark, _ := strings.Cut(rows[0], "\t")
		return remark
	}
	return ""
}
