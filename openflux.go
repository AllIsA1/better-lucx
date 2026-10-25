// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "OpenFlux in the subscription" module.

package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// The panel builds a subscription from the inbounds a client belongs to, chosen by a query that lists the
// protocols it knows. openflux is not on that list, so the link of an OpenFlux inbound never reaches the
// subscription even though the panel can build it. This module adds those links to the answer.
const (
	linkPrefix  = "openflux://"
	pageSize    = 200
	maxPages    = 50
	loadTimeout = 10 * time.Second
)

// fixer knows, per subscription id, the OpenFlux links its clients are entitled to.
type fixer struct {
	*cachedMap
	api panelAPI

	namesMu sync.Mutex
	names   map[string]string // OpenFlux link -> the remark of its inbound, for the page
}

func newFixer(api panelAPI) *fixer {
	f := &fixer{api: api}
	f.cachedMap = newCachedMap("openflux", f.load)
	return f
}

// nameOf is the remark of the OpenFlux inbound a link comes from, for the page of a browser.
func (f *fixer) nameOf(line string) string {
	if !strings.HasPrefix(strings.TrimSpace(line), linkPrefix) {
		return ""
	}
	f.namesMu.Lock()
	defer f.namesMu.Unlock()
	return f.names[strings.TrimSpace(line)]
}

// onSeam is the seam with this module alone (the plugin itself runs all modules through subscriptionSeam).
func (f *fixer) onSeam(c SeamCall) (SeamReply, error) { return subscriptionSeam(c, f) }

type inboundInfo struct {
	ID             int    `json:"id"`
	Remark         string `json:"remark"`
	Protocol       string `json:"protocol"`
	Enable         bool   `json:"enable"`
	ExcludeFromSub bool   `json:"excludeFromSub"`
}

type clientInfo struct {
	Email      string `json:"email"`
	SubID      string `json:"subId"`
	Enable     bool   `json:"enable"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	InboundIDs []int  `json:"inboundIds"`
	Traffic    *struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	} `json:"traffic"`
}

type clientPage struct {
	Items    []clientInfo `json:"items"`
	Filtered int          `json:"filtered"`
}

// load reads, from the panel, which OpenFlux inbounds are open to subscriptions and which clients belong to
// them, and returns the links per subscription id.
func (f *fixer) load(ctx context.Context) (map[string][]string, error) {
	var inbounds []inboundInfo
	if err := f.api.Do(ctx, http.MethodGet, "/inbounds/list/slim", nil, &inbounds); err != nil {
		return nil, err
	}
	open := map[int]bool{}
	remarkOf := map[int]string{}
	for _, ib := range inbounds {
		if ib.Protocol == "openflux" && ib.Enable && !ib.ExcludeFromSub {
			open[ib.ID] = true
			remarkOf[ib.ID] = strings.TrimSpace(ib.Remark)
		}
	}
	names := map[string]string{}
	defer func() {
		f.namesMu.Lock()
		f.names = names
		f.namesMu.Unlock()
	}()
	data := map[string][]string{}
	if len(open) == 0 {
		return data, nil
	}
	var clients []clientInfo
	for page := 1; page <= maxPages; page++ {
		var resp clientPage
		path := fmt.Sprintf("/clients/list/paged?protocol=openflux&page=%d&pageSize=%d", page, pageSize)
		if err := f.api.Do(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		clients = append(clients, resp.Items...)
		if len(resp.Items) == 0 || len(clients) >= resp.Filtered {
			break
		}
	}
	linkOf := map[int][]string{}
	nowMs := f.now().UnixMilli()
	for _, c := range clients {
		if c.SubID == "" || !usable(c, nowMs) {
			continue
		}
		ids := append([]int(nil), c.InboundIDs...)
		sort.Ints(ids)
		for _, id := range ids {
			if !open[id] {
				continue
			}
			links, seen := linkOf[id]
			if !seen {
				var all []string
				if err := f.api.Do(ctx, http.MethodGet, fmt.Sprintf("/inbounds/%d/links", id), nil, &all); err != nil {
					return nil, err
				}
				for _, l := range all {
					if strings.HasPrefix(l, linkPrefix) {
						links = append(links, l)
						if remarkOf[id] != "" {
							names[l] = remarkOf[id]
						}
					}
				}
				linkOf[id] = links
			}
			data[c.SubID] = appendNew(data[c.SubID], links)
		}
	}
	return data, nil
}

// usable: the panel gives a client that is switched off, expired or out of traffic no working link, so neither
// does this plugin.
func usable(c clientInfo, nowMs int64) bool {
	var used int64
	if c.Traffic != nil {
		used = c.Traffic.Up + c.Traffic.Down
	}
	return usableRecord(c.Enable, c.ExpiryTime, c.TotalGB, used, nowMs)
}

func usableRecord(enable bool, expiryMs, totalBytes, usedBytes, nowMs int64) bool {
	if !enable {
		return false
	}
	if expiryMs > 0 && expiryMs <= nowMs {
		return false
	}
	if totalBytes > 0 && usedBytes >= totalBytes {
		return false
	}
	return true
}

func appendNew(list, add []string) []string {
	for _, a := range add {
		dup := false
		for _, l := range list {
			dup = dup || l == a
		}
		if !dup {
			list = append(list, a)
		}
	}
	return list
}
