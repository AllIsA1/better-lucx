// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The subscription page of a browser: the panel draws its list of links from the data it embeds in the page
// (window.__SUB_PAGE_DATA__), which is built from the same query as the raw subscription. The links the
// modules add to the raw subscription are put into that list too, so the page and the proxy client agree, and
// the names the panel does not give (linkNames, one per link) are added next to it.

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
func pageSeam(r *SeamResponse, subID string, modules []subModule, namers []namer) SeamReply {
	body, changed := pageWithLinks(r.Body, subID, modules, namers)
	if !changed {
		return Keep()
	}
	return Modify(ResponseChange{Body: Bytes(body)})
}

func pageWithLinks(body []byte, subID string, modules []subModule, namers []namer) ([]byte, bool) {
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
	// What to call the links the panel names nothing: the page reads linkNames[i] for links[i].
	names := make([]string, len(lines))
	named := false
	for i, l := range lines {
		for _, n := range namers {
			if name := n.nameOf(l); name != "" {
				names[i], named = name, true
				break
			}
		}
	}
	if slices.Equal(lines, links) && !named {
		return body, false
	}
	rawLinks, err := json.Marshal(lines)
	if err != nil {
		return body, false
	}
	data["links"] = rawLinks
	if named {
		rawNames, err := json.Marshal(names)
		if err != nil {
			return body, false
		}
		data["linkNames"] = rawNames
	}
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
