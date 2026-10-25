// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "OpenFlux name" helper. The panel (lucx.313 on) puts the OpenFlux links into the subscription itself, but it
// names them nothing, so the page of a browser would call them "Link N". This tells the page the remark of the
// inbound a link belongs to. The lines themselves are not touched.

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	linkPrefix  = "openflux://"
	loadTimeout = 10 * time.Second
)

// openfluxKey is where the module keeps its rows in the cached map: "link\tremark" per OpenFlux link.
const openfluxKey = "openflux"

// openfluxNames knows the OpenFlux inbounds and names their links.
type openfluxNames struct {
	cm  *cachedMap
	api panelAPI
}

func newOpenfluxNames(api panelAPI) *openfluxNames {
	n := &openfluxNames{api: api}
	n.cm = newCachedMap("openflux", n.load)
	return n
}

type inboundInfo struct {
	ID             int    `json:"id"`
	Remark         string `json:"remark"`
	Protocol       string `json:"protocol"`
	Enable         bool   `json:"enable"`
	ExcludeFromSub bool   `json:"excludeFromSub"`
}

// load reads the OpenFlux inbounds that have a remark and their links.
func (n *openfluxNames) load(ctx context.Context) (map[string][]string, error) {
	var inbounds []inboundInfo
	if err := n.api.Do(ctx, http.MethodGet, "/inbounds/list/slim", nil, &inbounds); err != nil {
		return nil, err
	}
	var rows []string
	for _, ib := range inbounds {
		remark := strings.TrimSpace(ib.Remark)
		if ib.Protocol != "openflux" || !ib.Enable || ib.ExcludeFromSub || remark == "" {
			continue
		}
		var links []string
		if err := n.api.Do(ctx, http.MethodGet, fmt.Sprintf("/inbounds/%d/links", ib.ID), nil, &links); err != nil {
			return nil, err
		}
		for _, l := range links {
			if strings.HasPrefix(l, linkPrefix) {
				rows = append(rows, l+"\t"+remark)
			}
		}
	}
	return map[string][]string{openfluxKey: rows}, nil
}

// nameOf is the remark of the OpenFlux inbound a link comes from, "" for any other line.
func (n *openfluxNames) nameOf(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, linkPrefix) {
		return ""
	}
	for _, row := range n.cm.linksFor(openfluxKey) {
		if link, remark, _ := strings.Cut(row, "\t"); link == line {
			return remark
		}
	}
	return ""
}
