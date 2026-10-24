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
	fix   *fixer
	now   time.Time
}

func newRig(t *testing.T) *rig {
	r := &rig{t: t, panel: newPanel(), now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	r.panel.inbounds = []map[string]any{openflux(1), openflux(2), {"id": 3, "protocol": "vless", "enable": true}}
	r.panel.links = map[int][]string{1: {linkA}, 2: {linkB}, 3: {vless}}
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1, 3}), client("bob", "subB", []int{2})}
	r.fix = newFixer(r.panel)
	r.fix.now = func() time.Time { return r.now }
	r.fix.firstLoadWait = 2 * time.Second
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
	if reply, err := r.fix.onSeam(call(sub, body, headers)); err != nil || reply.Action != "keep" {
		r.t.Errorf("reply = %+v, err %v, want keep", reply, err)
	}
}

func TestAddsTheOpenFluxLinkTheSubscriptionLacks(t *testing.T) {
	r := newRig(t)
	reply, err := r.fix.onSeam(call("subA", b64(vless), nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.decoded(reply), vless+"\n"+linkA; got != want {
		t.Errorf("subscription = %q, want %q", got, want)
	}
}

func TestAddsToAPlainTextSubscriptionToo(t *testing.T) {
	r := newRig(t)
	hdr := map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}}
	reply, err := r.fix.onSeam(call("subA", []byte(vless+"\n"), hdr))
	if err != nil || reply.Action != "modify" || string(*reply.Response.Body) != vless+"\n"+linkA {
		t.Fatalf("reply = %+v, err %v", reply, err)
	}
}

func TestAddsEveryOpenFluxInboundOfTheSubscription(t *testing.T) {
	r := newRig(t)
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1, 2, 3})}
	reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
	if got, want := r.decoded(reply), vless+"\n"+linkA+"\n"+linkB; got != want {
		t.Errorf("subscription = %q, want %q", got, want)
	}
}

func TestSeveralClientsOfOneSubscriptionAreCovered(t *testing.T) {
	r := newRig(t)
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1}), client("alice2", "subA", []int{2})}
	reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
	if got, want := r.decoded(reply), vless+"\n"+linkA+"\n"+linkB; got != want {
		t.Errorf("subscription = %q", got)
	}
}

func TestDoesNotAddWhatTheSubscriptionAlreadyHas(t *testing.T) {
	r := newRig(t)
	r.keeps("subA", b64(vless+"\n"+linkA), nil) // the panel itself has been fixed
	hdr := map[string][]string{"Content-Type": {"text/plain"}}
	r.keeps("subA", []byte(vless+"\n"+linkA+"\n"), hdr)
}

func TestLeavesAlonewhatItDoesNotKnow(t *testing.T) {
	r := newRig(t)
	r.keeps("nobody", b64(vless), nil)                                                    // no such subscription
	r.keeps("subA", []byte("<html>page</html>"), nil)                                     // not a subscription
	r.keeps("subA", nil, nil)                                                             // empty
	r.keeps("subA", []byte("hello"), map[string][]string{"Content-Type": {"text/plain"}}) // plain text, no links
	c := call("subA", b64(vless), nil)
	c.Response.Status = 404
	if reply, _ := r.fix.onSeam(c); reply.Action != "keep" {
		t.Errorf("an error answer was changed: %+v", reply)
	}
	c = call("subA", b64(vless), nil)
	c.Response = nil
	if reply, _ := r.fix.onSeam(c); reply.Action != "keep" {
		t.Errorf("a call without an answer was changed: %+v", reply)
	}
	c = call("", b64(vless), nil)
	c.Request.Params = nil
	if reply, _ := r.fix.onSeam(c); reply.Action != "keep" {
		t.Errorf("a call without a subscription id was changed: %+v", reply)
	}
}

func TestOnlyTheRouteItDeclaredIsTouched(t *testing.T) {
	r := newRig(t)
	c := call("subA", b64(vless), nil)
	c.Seam.Route = "/json/:subid"
	if reply, _ := r.fix.onSeam(c); reply.Action != "keep" {
		t.Errorf("another format was changed: %+v", reply)
	}
	c = call("subA", b64(vless), nil)
	c.Seam.Mode = "before"
	if _, err := r.fix.onSeam(c); err == nil {
		t.Error("a mode the plugin never declared must fail")
	}
}

func TestKeepsOutWhatThePanelHidesFromSubscriptions(t *testing.T) {
	cases := map[string]func(r *rig){
		"inbound switched off":       func(r *rig) { r.panel.inbounds[0]["enable"] = false },
		"inbound excluded from subs": func(r *rig) { r.panel.inbounds[0]["excludeFromSub"] = true },
		"client switched off":        func(r *rig) { r.panel.clients[0]["enable"] = false },
		"client expired":             func(r *rig) { r.panel.clients[0]["expiryTime"] = r.now.Add(-time.Hour).UnixMilli() },
		"client's traffic used up":   func(r *rig) { r.panel.clients[0]["totalGB"] = 30 },
		"client not on the inbound":  func(r *rig) { r.panel.clients[0]["inboundIds"] = []int{3} },
		"the panel has no link":      func(r *rig) { r.panel.links[1] = nil },
		"a link of another protocol": func(r *rig) { r.panel.links[1] = []string{"trojan://x@host.example:443#t"} },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			setup(r)
			r.keeps("subA", b64(vless), nil)
		})
	}
}

func TestAClientWithTimeLeftAndTrafficLeftIsServed(t *testing.T) {
	r := newRig(t)
	r.panel.clients[0]["expiryTime"] = r.now.Add(time.Hour).UnixMilli()
	r.panel.clients[0]["totalGB"] = 31
	reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
	if got := r.decoded(reply); !strings.HasSuffix(got, linkA) {
		t.Errorf("subscription = %q", got)
	}
}

func TestAsksThePanelOnlyAboutOpenFluxAndOnlyReadsIt(t *testing.T) {
	r := newRig(t)
	r.fix.onSeam(call("subA", b64(vless), nil))
	r.panel.mu.Lock()
	defer r.panel.mu.Unlock()
	for route := range r.panel.calls {
		if route != "/inbounds/list/slim" && route != "/clients/list/paged" && !(strings.HasPrefix(route, "/inbounds/") && strings.HasSuffix(route, "/links")) {
			t.Errorf("the plugin called %s", route)
		}
	}
	if r.panel.calls["/inbounds/3/links"] != 0 {
		t.Error("the links of an inbound that is not OpenFlux were fetched")
	}
}

func TestTheAnswerComesFromMemoryAfterTheFirstLoad(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 5; i++ {
		r.fix.onSeam(call("subA", b64(vless), nil))
	}
	if n := r.panel.count("/inbounds/list/slim"); n != 1 {
		t.Errorf("the inbound list was fetched %d times for 5 requests", n)
	}
	if n := r.panel.count("/clients/list/paged"); n != 1 {
		t.Errorf("the client list was fetched %d times for 5 requests", n)
	}
}

func TestAStaleAnswerIsServedAtOnceWhileTheDataIsRefreshed(t *testing.T) {
	r := newRig(t)
	r.fix.onSeam(call("subA", b64(vless), nil))
	r.panel.mu.Lock()
	r.panel.links[1] = []string{"openflux://changed@host.example:7001/"}
	r.panel.mu.Unlock()
	r.now = r.now.Add(2 * r.fix.ttl)
	reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
	if got := r.decoded(reply); !strings.HasSuffix(got, linkA) {
		t.Errorf("a stale answer was expected at once, got %q", got)
	}
	waitFor(t, func() bool { return r.panel.count("/inbounds/list/slim") == 2 })
	waitFor(t, func() bool {
		reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
		return reply.Action == "modify" && strings.Contains(string(mustDecode(*reply.Response.Body)), "changed@")
	})
}

func mustDecode(b []byte) []byte { raw, _ := base64.StdEncoding.DecodeString(string(b)); return raw }

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

func TestAPanelThatFailsLeavesTheSubscriptionAloneAndIsRetried(t *testing.T) {
	r := newRig(t)
	r.panel.fail = errors.New("panel answered HTTP 500")
	r.keeps("subA", b64(vless), nil)
	r.panel.mu.Lock()
	r.panel.fail = nil
	r.panel.mu.Unlock()
	r.now = r.now.Add(r.fix.retry + time.Second)
	waitFor(t, func() bool {
		reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
		return reply.Action == "modify"
	})
}

func TestAFailingPanelIsNotHammered(t *testing.T) {
	r := newRig(t)
	r.panel.fail = errors.New("down")
	for i := 0; i < 20; i++ {
		r.fix.onSeam(call("subA", b64(vless), nil))
	}
	if n := r.panel.count("/inbounds/list/slim"); n > 2 {
		t.Errorf("a failing panel was asked %d times in 20 requests", n)
	}
}

func TestAFirstRequestDoesNotWaitLongerThanItMay(t *testing.T) {
	r := newRig(t)
	r.fix.firstLoadWait = 50 * time.Millisecond
	r.panel.slow = 300 * time.Millisecond
	start := time.Now()
	r.keeps("subA", b64(vless), nil)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("the first request waited %v", d)
	}
	waitFor(t, func() bool {
		reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
		return reply.Action == "modify"
	})
}

func TestManyClientsAreReadPageByPage(t *testing.T) {
	r := newRig(t)
	r.panel.clients = nil
	for i := 0; i < 450; i++ {
		r.panel.clients = append(r.panel.clients, client(fmt.Sprintf("u%d", i), fmt.Sprintf("s%d", i), []int{1}))
	}
	reply, _ := r.fix.onSeam(call("s449", b64(vless), nil))
	if got := r.decoded(reply); !strings.HasSuffix(got, linkA) {
		t.Errorf("the last client of several pages was missed: %q", got)
	}
	if n := r.panel.count("/clients/list/paged"); n < 2 || n > 5 {
		t.Errorf("pages fetched = %d", n)
	}
}

func TestNoOpenFluxInboundsMeansNoWork(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = []map[string]any{{"id": 3, "protocol": "vless", "enable": true}}
	r.keeps("subA", b64(vless), nil)
	if n := r.panel.count("/clients/list/paged"); n != 0 {
		t.Errorf("clients were listed although there is no OpenFlux inbound (%d)", n)
	}
}

func TestRequestsThatArriveTogetherShareOneLoad(t *testing.T) {
	r := newRig(t)
	r.panel.slow = 100 * time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.fix.onSeam(call("subA", b64(vless), nil))
		}()
	}
	wg.Wait()
	if n := r.panel.count("/inbounds/list/slim"); n != 1 {
		t.Errorf("the panel was read %d times for 6 simultaneous requests", n)
	}
}

func TestAClientListThatPromisesMoreThanItHoldsEndsWithTheLastPage(t *testing.T) {
	r := newRig(t)
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1})}
	r.panel.inflate = 100000
	reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
	if reply.Action != "modify" {
		t.Fatalf("reply = %+v", reply)
	}
	if n := r.panel.count("/clients/list/paged"); n > 3 {
		t.Errorf("%d pages were asked for one client", n)
	}
}

func TestTheLinksOfOneInboundAreFetchedOnceForManyClients(t *testing.T) {
	r := newRig(t)
	r.panel.clients = []map[string]any{client("a", "subA", []int{1}), client("b", "subB", []int{1}), client("c", "subC", []int{1})}
	r.fix.onSeam(call("subA", b64(vless), nil))
	if n := r.panel.count("/inbounds/1/links"); n != 1 {
		t.Errorf("the links of one inbound were fetched %d times", n)
	}
}

func TestLinksComeInTheOrderOfTheInbounds(t *testing.T) {
	r := newRig(t)
	r.panel.clients = []map[string]any{client("alice", "subA", []int{2, 1})}
	reply, _ := r.fix.onSeam(call("subA", b64(vless), nil))
	if got, want := r.decoded(reply), vless+"\n"+linkA+"\n"+linkB; got != want {
		t.Errorf("subscription = %q, want %q", got, want)
	}
}

func TestPlainTextWithoutTheRightContentTypeIsNotASubscription(t *testing.T) {
	r := newRig(t)
	r.keeps("subA", []byte(vless+"\n"), nil)
	r.keeps("subA", []byte(vless+"\n"), map[string][]string{"Content-Type": {"text/html"}})
}
