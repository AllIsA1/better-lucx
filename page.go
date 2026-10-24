// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "subscription page" module: the page a client's browser shows for a subscription link gets one more
// card, "AmneziaWG link format", where the form of each AmneziaWG link can be chosen and copied. The page is
// drawn by the panel's own script; the card is added to its HTML, with its data and a small script of its own,
// and puts itself under the panel's card once that is on the screen.

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed page/block.js
var pageScript string

//go:embed page/block.css
var pageStyle string

const (
	pageMarker = `data-better-lucx="awg-format"`
	dataID     = "better-lucx-awg-data"
	pageData   = "window.__SUB_PAGE_DATA__="
)

var confLinkLine = regexp.MustCompile(`^amneziawg://[A-Za-z0-9_-]+(#.*)?$`)

// pageFormat is one form of an AmneziaWG link: its kind and the text to copy.
type pageFormat struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// pageNode is one AmneziaWG node of a subscription with every form it can be had in.
type pageNode struct {
	Name    string       `json:"name"`
	Formats []pageFormat `json:"formats"`
}

func isHTML(r *SeamResponse) bool {
	for name, values := range r.Headers {
		if strings.EqualFold(name, "Content-Type") {
			for _, v := range values {
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "text/html") {
					return true
				}
			}
		}
	}
	return false
}

// subPageLinks reads the subscription's lines out of the data the panel writes into its page.
func subPageLinks(html string) ([]string, bool) {
	i := strings.Index(html, pageData)
	if i < 0 {
		return nil, false
	}
	var data struct {
		Links []string `json:"links"`
	}
	if err := json.NewDecoder(strings.NewReader(html[i+len(pageData):])).Decode(&data); err != nil || data.Links == nil {
		return nil, false
	}
	return data.Links, true
}

// awgPageNodes finds the AmneziaWG nodes in the lines of a subscription: every vpn:// line that holds a
// config, with the older amneziawg:// line right before it (if it has one).
func awgPageNodes(links []string) []pageNode {
	var nodes []pageNode
	for i, line := range links {
		if !strings.HasPrefix(line, "vpn://") {
			continue
		}
		conf := vpnConf(line)
		if conf == "" {
			continue
		}
		name := awgConfName(conf)
		if name == "" {
			name = fmt.Sprintf("AmneziaWG %d", len(nodes)+1)
		}
		formats := []pageFormat{{ID: "vpn", Value: line}, {ID: "conf-link", Value: awgConfLink(conf)}}
		if i > 0 && strings.HasPrefix(links[i-1], "amneziawg://") && !confLinkLine.MatchString(links[i-1]) {
			formats = append(formats, pageFormat{ID: "legacy", Value: links[i-1]})
		}
		formats = append(formats, pageFormat{ID: "conf", Value: conf})
		nodes = append(nodes, pageNode{Name: name, Formats: formats})
	}
	return nodes
}

// injectPageBlock adds the card to a subscription page, once.
func injectPageBlock(html string, nodes []pageNode) string {
	if strings.Contains(html, pageMarker) {
		return html
	}
	data, err := json.Marshal(nodes) // escapes < > & and the line separators: it cannot end its script early
	if err != nil {
		return html
	}
	block := "<style " + pageMarker + ">" + pageStyle + "</style>" +
		`<script type="application/json" id="` + dataID + `" ` + pageMarker + ">" + string(data) + "</script>" +
		"<script " + pageMarker + ">" + pageScript + "</script>"
	if i := strings.LastIndex(strings.ToLower(html), "</body>"); i >= 0 {
		return html[:i] + block + html[i:]
	}
	return html + block
}

// pageBlockSeam answers the seam for a response that is the subscription page of a browser, not a subscription.
func pageBlockSeam(r *SeamResponse) SeamReply {
	html := string(r.Body)
	links, ok := subPageLinks(html)
	if !ok {
		return Keep()
	}
	nodes := awgPageNodes(links)
	if len(nodes) == 0 {
		return Keep()
	}
	out := injectPageBlock(html, nodes)
	if out == html {
		return Keep()
	}
	return Modify(ResponseChange{Body: Bytes([]byte(out))})
}
