// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

const seamRoute = "/links/:subid"

// linkSource is a module that adds lines to a subscription.
type linkSource interface {
	linksFor(subID string) []string
}

// subscriptionSeam answers the seam declared in lucx-plugin.json: the lines every source has for the
// subscription are added to the panel's answer, unless the answer already has them.
func subscriptionSeam(c SeamCall, sources ...linkSource) (SeamReply, error) {
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
	text, wrapped, ok := subscriptionText(c.Response)
	if !ok {
		return Keep(), nil
	}
	present := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, src := range sources {
		for _, link := range src.linksFor(subID) {
			if !present[link] {
				present[link] = true
				missing = append(missing, link)
			}
		}
	}
	if len(missing) == 0 {
		return Keep(), nil
	}
	changed := strings.TrimRight(text, "\n") + "\n" + strings.Join(missing, "\n")
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
