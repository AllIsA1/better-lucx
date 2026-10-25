// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The subscription page of a browser: the panel draws its list of links from the data it embeds in the page
// (window.__SUB_PAGE_DATA__), which is built from the same query as the raw subscription. The links the
// modules add to the raw subscription are put into that list too, so the page and the proxy client agree.

package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

const pageDataMarker = "window.__SUB_PAGE_DATA__="

// isHTML says whether the answer is the page a browser shows.
func isHTML(r *SeamResponse) bool {
	for name, values := range r.Headers {
		if !strings.EqualFold(name, "Content-Type") {
			continue
		}
		for _, v := range values {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "text/html") {
				return true
			}
		}
	}
	return false
}

// pageSeam gives the page's link list the modules' lines. It leaves the page alone when anything about it is
// not as expected (a custom theme without the data, a changed format).
func pageSeam(r *SeamResponse, subID string, modules []subModule) SeamReply {
	body, changed := pageWithLinks(r.Body, subID, modules)
	if !changed {
		return Keep()
	}
	return Modify(ResponseChange{Body: Bytes(body)})
}

func pageWithLinks(body []byte, subID string, modules []subModule) ([]byte, bool) {
	at := bytes.Index(body, []byte(pageDataMarker))
	if at < 0 {
		return body, false
	}
	start := at + len(pageDataMarker)
	dec := json.NewDecoder(bytes.NewReader(body[start:]))
	var data map[string]json.RawMessage
	if err := dec.Decode(&data); err != nil {
		return body, false
	}
	end := start + int(dec.InputOffset())
	var links []string
	if raw, ok := data["links"]; !ok || json.Unmarshal(raw, &links) != nil {
		return body, false
	}
	lines := slices.Clone(links)
	for _, m := range modules {
		lines = m.apply(subID, lines)
	}
	if slices.Equal(lines, links) {
		return body, false
	}
	rawLinks, err := json.Marshal(lines)
	if err != nil {
		return body, false
	}
	data["links"] = rawLinks
	out, err := json.Marshal(data)
	if err != nil {
		return body, false
	}
	var b bytes.Buffer
	b.Grow(len(body) + len(out))
	b.Write(body[:start])
	b.Write(out)
	b.Write(body[end:])
	return b.Bytes(), true
}
