// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const seamRoute = "/links/:subid"

// subModule is a part of the plugin that changes a subscription: it gets the lines of the panel's answer
// (and what the modules before it made of them) and returns the lines to send.
type subModule interface {
	apply(subID string, lines []string) []string
}

// appendMissing adds the lines that are not there yet at the end.
func appendMissing(lines, extra []string) []string {
	present := make(map[string]bool, len(lines)+len(extra))
	for _, l := range lines {
		present[strings.TrimSpace(l)] = true
	}
	for _, l := range extra {
		if !present[l] {
			present[l] = true
			lines = append(lines, l)
		}
	}
	return lines
}

// namer tells the page of a browser what to call a link the panel gives no name (the name is not put in the
// link: some apps read the whole line after the scheme and drop a link that carries a #name).
type namer interface {
	nameOf(line string) string
}

// subscriptionSeam answers the seam declared in lucx-plugin.json: every module gets its turn at the
// subscription, in the order given. Each of the parts is a subModule, a namer or both.
func subscriptionSeam(c SeamCall, parts ...any) (SeamReply, error) {
	var modules []subModule
	var namers []namer
	var raws []rawOnly
	for _, p := range parts {
		if m, ok := p.(rawOnly); ok {
			raws = append(raws, m)
		}
		if m, ok := p.(subModule); ok {
			modules = append(modules, m)
		}
		if n, ok := p.(namer); ok {
			namers = append(namers, n)
		}
	}
	if c.Seam.Engine != "sub" || c.Seam.Mode != "after" {
		return SeamReply{}, fmt.Errorf("no seam %s/%s/%s is declared", c.Seam.Engine, c.Seam.Mode, c.Seam.Route)
	}
	if c.Seam.Route != seamRoute || c.Response == nil || c.Response.Status != http.StatusOK || len(c.Response.Body) == 0 {
		return Keep(), nil
	}
	subID := c.Request.Params["subid"]
	if subID == "" {
		return Keep(), nil
	}
	if isHTML(c.Response) {
		return pageSeam(c.Response, subID, modules, namers), nil // the page of a browser: its list of links
	}
	text, wrapped, ok := subscriptionText(c.Response)
	if !ok {
		return Keep(), nil
	}
	original := strings.Split(strings.TrimRight(text, "\n"), "\n")
	lines := append([]string(nil), original...)
	for _, m := range modules {
		lines = m.apply(subID, lines)
	}
	for _, m := range raws {
		lines = m.applyRaw(lines)
	}
	if slices.Equal(lines, original) {
		return Keep(), nil
	}
	changed := strings.Join(lines, "\n")
	if wrapped {
		changed = base64.StdEncoding.EncodeToString([]byte(changed))
	}
	return Modify(ResponseChange{Body: Bytes([]byte(changed))}), nil
}

// subscriptionText returns the lines of a subscription: base64 of one link per line, or the same as plain
// text (the panel's "encrypt subscription" setting off). Anything else, an HTML page for instance, is not one.
func subscriptionText(r *SeamResponse) (text string, wrapped, ok bool) {
	if decoded, err := base64.StdEncoding.DecodeString(string(r.Body)); err == nil {
		return string(decoded), true, true
	}
	for name, values := range r.Headers {
		if !strings.EqualFold(name, "Content-Type") {
			continue
		}
		for _, v := range values {
			if strings.HasPrefix(strings.ToLower(v), "text/plain") && strings.Contains(string(r.Body), "://") {
				return string(r.Body), false, true
			}
		}
	}
	return "", false, false
}
