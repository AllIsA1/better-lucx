// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	linkA = "openflux://a-secret@host.example:7001/?name=A"
	linkB = "openflux://b-secret@host.example:7002/?name=B"
	vless = "vless://u@host.example:443?type=tcp#one"
)

// fakePanel answers the three panel routes the plugin uses.
type fakePanel struct {
	mu        sync.Mutex
	inbounds  []map[string]any
	clients   []map[string]any
	links     map[int][]string
	fail      error
	calls     map[string]int
	slow      time.Duration
	inflate   int                       // pretend to hold more clients than there are
	byEmail   map[string]map[string]any // answers of /clients/get/<email>
	lastPaged string                    // the last /clients/list/paged request, as it was sent
}

func newPanel() *fakePanel { return &fakePanel{links: map[int][]string{}, calls: map[string]int{}} }

func (p *fakePanel) Do(ctx context.Context, method, path string, in, out any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	route := path
	if i := strings.IndexByte(route, '?'); i >= 0 {
		route = route[:i]
	}
	p.calls[route]++
	if p.slow > 0 {
		p.mu.Unlock()
		time.Sleep(p.slow)
		p.mu.Lock()
	}
	if p.fail != nil {
		return p.fail
	}
	var obj any
	switch {
	case route == "/inbounds/list/slim":
		obj = p.inbounds
	case strings.HasPrefix(route, "/clients/get/"):
		email, _ := url.PathUnescape(strings.TrimPrefix(route, "/clients/get/"))
		c, ok := p.byEmail[email]
		if !ok {
			return errors.New("record not found")
		}
		obj = c
	case route == "/clients/list/paged":
		q, _ := url.ParseQuery(strings.SplitN(path, "?", 2)[1])
		if q.Get("protocol") == "" {
			p.lastPaged = path
			var found []map[string]any
			for _, c := range p.clients {
				if strings.Contains(fmt.Sprint(c["email"]), q.Get("search")) {
					found = append(found, c)
				}
			}
			raw, _ := json.Marshal(map[string]any{"items": found, "total": len(p.clients), "filtered": len(found), "page": 1, "pageSize": 20})
			return json.Unmarshal(raw, out)
		}
		page, _ := strconv.Atoi(q.Get("page"))
		size, _ := strconv.Atoi(q.Get("pageSize"))
		if page < 1 {
			page = 1
		}
		lo, hi := (page-1)*size, page*size
		if lo > len(p.clients) {
			lo = len(p.clients)
		}
		if hi > len(p.clients) {
			hi = len(p.clients)
		}
		obj = map[string]any{"items": p.clients[lo:hi], "total": len(p.clients), "filtered": max(len(p.clients), p.inflate), "page": page, "pageSize": size}
	case strings.HasPrefix(route, "/inbounds/") && strings.HasSuffix(route, "/links"):
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(route, "/inbounds/"), "/links"))
		obj = p.links[id]
	default:
		return fmt.Errorf("the plugin asked for %s %s, which it has no business with", method, path)
	}
	raw, _ := json.Marshal(obj)
	return json.Unmarshal(raw, out)
}

func (p *fakePanel) count(route string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[route]
}

func openflux(id int, mutate ...func(m map[string]any)) map[string]any {
	m := map[string]any{"id": id, "protocol": "openflux", "enable": true, "excludeFromSub": false}
	for _, f := range mutate {
		f(m)
	}
	return m
}

func client(email, sub string, inbounds []int, mutate ...func(m map[string]any)) map[string]any {
	m := map[string]any{"email": email, "subId": sub, "enable": true, "totalGB": 0, "expiryTime": 0, "inboundIds": inbounds,
		"traffic": map[string]any{"up": 10, "down": 20}}
	for _, f := range mutate {
		f(m)
	}
	return m
}

type rig struct {
	t     *testing.T
	panel *fakePanel
	fix   *openfluxNames
	now   time.Time
}

func newRig(t *testing.T) *rig {
	r := &rig{t: t, panel: newPanel(), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	r.panel.inbounds = []map[string]any{openflux(1), openflux(2), {"id": 3, "protocol": "vless", "enable": true}}
	r.panel.links = map[int][]string{1: {linkA}, 2: {linkB}, 3: {vless}}
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1, 3}), client("bob", "subB", []int{2})}
	r.fix = newOpenfluxNames(r.panel)
	r.fix.cm.now = func() time.Time { return r.now }
	r.fix.cm.firstLoadWait = 2 * time.Second
	return r
}

func b64(s string) []byte { return []byte(base64.StdEncoding.EncodeToString([]byte(s))) }

func call(sub string, body []byte, headers map[string][]string) SeamCall {
	var c SeamCall
	c.Seam.Engine, c.Seam.Mode, c.Seam.Route = "sub", "after", "/links/:subid"
	c.Request = SeamRequest{Method: "GET", Route: "/links/:subid", Params: map[string]string{"subid": sub}}
	c.Response = &SeamResponse{Status: 200, Body: body, Headers: headers}
	return c
}

func (r *rig) decoded(reply SeamReply) string {
	r.t.Helper()
	if reply.Action != "modify" || reply.Response == nil || reply.Response.Body == nil {
		r.t.Fatalf("reply = %+v, want a modified body", reply)
	}
	raw, err := base64.StdEncoding.DecodeString(string(*reply.Response.Body))
	if err != nil {
		r.t.Fatalf("the changed subscription is not base64: %v", err)
	}
	return string(raw)
}

func (r *rig) keeps(sub string, body []byte, headers map[string][]string) {
	r.t.Helper()
	if reply, err := subscriptionSeam(call(sub, body, headers), awgFormat{}); err != nil || reply.Action != "keep" {
		r.t.Errorf("reply = %+v, err %v, want keep", reply, err)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

// openfluxNamerRig is a namer over a panel with one OpenFlux inbound holding the given link, named remark.
func openfluxNamerRig(t *testing.T, link, remark string) *openfluxNames {
	t.Helper()
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = remark })}
	r.panel.links = map[int][]string{1: {link}}
	return r.fix
}

func TestTheOpenFluxLinksKnowTheRemarkOfTheirInboundForThePage(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{
		openflux(1, func(m map[string]any) { m["remark"] = "  Польша OpenFlux " }),
		openflux(2),
		{"id": 3, "protocol": "vless", "enable": true, "remark": "V"},
	}
	if got := r.fix.nameOf(" " + linkA + " "); got != "Польша OpenFlux" {
		t.Errorf("nameOf(linkA) = %q", got)
	}
	if got := r.fix.nameOf(linkB); got != "" {
		t.Errorf("an inbound with no remark gives no name: %q", got)
	}
	if got := r.fix.nameOf(vless); got != "" {
		t.Errorf("only OpenFlux links are named here: %q", got)
	}
}

func TestNoOpenFluxNameForAHiddenOrSwitchedOffInbound(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{
		openflux(1, func(m map[string]any) { m["remark"] = "Hidden"; m["excludeFromSub"] = true }),
		openflux(2, func(m map[string]any) { m["remark"] = "Off"; m["enable"] = false }),
	}
	if a, b := r.fix.nameOf(linkA), r.fix.nameOf(linkB); a != "" || b != "" {
		t.Errorf("names = %q, %q", a, b)
	}
}

func TestTheRawSubscriptionKeepsTheOpenFluxLinkAsThePanelMadeIt(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = "Польша OpenFlux" })}
	r.keeps("subA", b64(vless+"\n"+linkA), nil)
	r.keeps("subA", b64(vless), nil) // and it is not added when the panel left it out: the panel does that itself now
}

func TestOnlyOpenFluxIsAskedAboutAndNothingIsWritten(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1), openflux(2), {"id": 3, "protocol": "vless", "enable": true, "remark": "V"}}
	r.fix.nameOf(linkA)
	r.panel.mu.Lock()
	defer r.panel.mu.Unlock()
	for route := range r.panel.calls {
		if route != "/inbounds/list/slim" && !(strings.HasPrefix(route, "/inbounds/") && strings.HasSuffix(route, "/links")) {
			t.Errorf("the plugin called %s", route)
		}
	}
	if r.panel.calls["/inbounds/3/links"] != 0 {
		t.Error("the links of an inbound that is not OpenFlux were fetched")
	}
}

func TestTheNamesComeFromMemoryAfterTheFirstLoad(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = "A" })}
	for i := 0; i < 5; i++ {
		r.fix.nameOf(linkA)
	}
	if n := r.panel.count("/inbounds/list/slim"); n != 1 {
		t.Errorf("the inbound list was fetched %d times for 5 lookups", n)
	}
}

func TestAStaleNameIsServedAtOnceWhileTheDataIsRefreshed(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = "Old" })}
	r.fix.nameOf(linkA)
	r.panel.mu.Lock()
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = "New" })}
	r.panel.mu.Unlock()
	r.now = r.now.Add(2 * r.fix.cm.ttl)
	if got := r.fix.nameOf(linkA); got != "Old" {
		t.Errorf("a stale name was expected at once, got %q", got)
	}
	waitFor(t, func() bool { return r.fix.nameOf(linkA) == "New" })
}

func TestAPanelThatFailsGivesNoNameAndIsRetriedButNotHammered(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = "A" })}
	r.panel.fail = errors.New("panel answered HTTP 500")
	for i := 0; i < 20; i++ {
		if got := r.fix.nameOf(linkA); got != "" {
			t.Fatalf("name = %q from a failing panel", got)
		}
	}
	if n := r.panel.count("/inbounds/list/slim"); n > 2 {
		t.Errorf("a failing panel was asked %d times in 20 lookups", n)
	}
	r.panel.mu.Lock()
	r.panel.fail = nil
	r.panel.mu.Unlock()
	r.now = r.now.Add(r.fix.cm.retry + time.Second)
	waitFor(t, func() bool { return r.fix.nameOf(linkA) == "A" })
}

func TestAFirstLookupDoesNotWaitLongerThanItMay(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{openflux(1, func(m map[string]any) { m["remark"] = "A" })}
	r.fix.cm.firstLoadWait = 50 * time.Millisecond
	r.panel.slow = 300 * time.Millisecond
	start := time.Now()
	r.fix.nameOf(linkA)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("the first lookup waited %v", d)
	}
	waitFor(t, func() bool { return r.fix.nameOf(linkA) == "A" })
}

func TestLookupsThatArriveTogetherShareOneLoad(t *testing.T) {
	r := newRig(t)
	r.panel.slow = 100 * time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.fix.nameOf(linkA)
		}()
	}
	wg.Wait()
	if n := r.panel.count("/inbounds/list/slim"); n != 1 {
		t.Errorf("the panel was read %d times for 6 simultaneous lookups", n)
	}
}
