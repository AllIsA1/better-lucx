// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiRig struct {
	*rig
	custom *customLinks
	mux    http.Handler
}

func newAPIRig(t *testing.T) *apiRig {
	r, c := customRig(t)
	return &apiRig{rig: r, custom: c, mux: newMux(r.panel, r.fix, c)}
}

func (a *apiRig) do(method, path string, body any, user string) (int, map[string]any) {
	a.t.Helper()
	var rdr *bytes.Reader
	if s, ok := body.(string); ok {
		rdr = bytes.NewReader([]byte(s))
	} else {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rdr)
	if user != "" {
		req.Header.Set("X-LucX-User", user)
	}
	rec := httptest.NewRecorder()
	a.mux.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestTheStateOfTheStoreIsReadable(t *testing.T) {
	a := newAPIRig(t)
	a.custom.store.add("alice", []newLink{{URL: "yptun://1"}, {URL: "yptun://2"}})
	code, out := a.do("GET", "/api/overview", nil, "admin")
	rows, _ := out["clients"].([]any)
	if code != 200 || len(rows) != 1 || rows[0].(map[string]any)["email"] != "alice" || rows[0].(map[string]any)["links"] != float64(2) {
		t.Errorf("overview = %d %v", code, out)
	}
	code, out = a.do("GET", "/api/links?email=alice", nil, "admin")
	links, _ := out["links"].([]any)
	if code != 200 || len(links) != 2 || links[0].(map[string]any)["url"] != "yptun://1" || links[0].(map[string]any)["enabled"] != true {
		t.Errorf("links = %d %v", code, out)
	}
	if code, _ := a.do("GET", "/api/links", nil, "admin"); code != 400 {
		t.Errorf("a request without a client = %d", code)
	}
}

func TestClientsCanBeSearchedWithTheirLinkCount(t *testing.T) {
	a := newAPIRig(t)
	a.custom.store.add("alice", []newLink{{URL: "yptun://1"}})
	code, out := a.do("GET", "/api/clients?search=ali", nil, "admin")
	items, _ := out["clients"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("clients = %d %v", code, out)
	}
	first := items[0].(map[string]any)
	if first["email"] != "alice" || first["subId"] != "subA" || first["links"] != float64(1) || first["enable"] != true {
		t.Errorf("client = %v", first)
	}
}

func TestOnlyAnAdminCanChangeLinks(t *testing.T) {
	a := newAPIRig(t)
	for _, path := range []string{"/api/links", "/api/links/update", "/api/links/delete"} {
		if code, _ := a.do("POST", path, map[string]any{"email": "alice", "text": "yptun://1"}, ""); code != http.StatusForbidden {
			t.Errorf("POST %s without a panel user = %d", path, code)
		}
	}
	if len(a.custom.store.list("alice")) != 0 {
		t.Error("a link was stored without a user")
	}
}

func TestLinksAreAddedFromAPastedList(t *testing.T) {
	a := newAPIRig(t)
	body := map[string]any{"email": "alice", "text": "yptun://one\n\n  qwdtt://two  \nnot a link\nyptun://one\n", "name": "Group"}
	code, out := a.do("POST", "/api/links", body, "admin")
	added, _ := out["added"].([]any)
	rejected, _ := out["rejected"].([]any)
	if code != 200 || len(added) != 2 || len(rejected) != 2 {
		t.Fatalf("add = %d %v", code, out)
	}
	got := a.custom.store.list("alice")
	if len(got) != 2 || got[0].URL != "yptun://one" || got[1].URL != "qwdtt://two" || got[0].Name != "Group" {
		t.Errorf("stored = %+v", got)
	}
	if rej := rejected[0].(map[string]any); rej["line"] != "not a link" || rej["reason"] == "" {
		t.Errorf("rejection = %v", rej)
	}
}

func TestLinksAreAddedWithTheirOwnNames(t *testing.T) {
	a := newAPIRig(t)
	body := map[string]any{"email": "alice", "links": []map[string]string{{"url": "yptun://one", "name": "One"}, {"url": "yptun://two"}}}
	if code, out := a.do("POST", "/api/links", body, "admin"); code != 200 || len(out["added"].([]any)) != 2 {
		t.Fatalf("add = %d %v", code, out)
	}
	if got := a.custom.store.list("alice"); got[0].Name != "One" || got[1].Name != "" {
		t.Errorf("stored = %+v", got)
	}
}

func TestLinksForAClientThePanelDoesNotKnowAreRefused(t *testing.T) {
	a := newAPIRig(t)
	if code, _ := a.do("POST", "/api/links", map[string]any{"email": "nobody", "text": "yptun://1"}, "admin"); code != http.StatusNotFound {
		t.Errorf("unknown client = %d", code)
	}
	if code, _ := a.do("POST", "/api/links", map[string]any{"email": "", "text": "yptun://1"}, "admin"); code != 400 {
		t.Errorf("no client = %d", code)
	}
	if code, _ := a.do("POST", "/api/links", "{broken", "admin"); code != 400 {
		t.Errorf("broken body = %d", code)
	}
	if len(a.custom.store.emails()) != 0 {
		t.Error("something was stored")
	}
}

func TestALinkIsSwitchedRenamedAndDeleted(t *testing.T) {
	a := newAPIRig(t)
	added, _, _ := a.custom.store.add("alice", []newLink{{URL: "yptun://1"}, {URL: "yptun://2"}})
	off := false
	code, _ := a.do("POST", "/api/links/update", map[string]any{"email": "alice", "id": added[0].ID, "enabled": off, "name": "N"}, "admin")
	if code != 200 {
		t.Fatalf("update = %d", code)
	}
	if got := a.custom.store.list("alice")[0]; got.Enabled || got.Name != "N" {
		t.Errorf("after update: %+v", got)
	}
	// only what is sent changes
	if code, _ := a.do("POST", "/api/links/update", map[string]any{"email": "alice", "id": added[0].ID, "enabled": true}, "admin"); code != 200 {
		t.Fatal(code)
	}
	if got := a.custom.store.list("alice")[0]; !got.Enabled || got.Name != "N" {
		t.Errorf("a partial update changed more: %+v", got)
	}
	if code, _ := a.do("POST", "/api/links/update", map[string]any{"email": "alice", "id": "nope", "enabled": true}, "admin"); code != 404 {
		t.Errorf("unknown link = %d", code)
	}
	if code, _ := a.do("POST", "/api/links/delete", map[string]any{"email": "alice", "id": added[1].ID}, "admin"); code != 200 {
		t.Errorf("delete = %d", code)
	}
	if code, _ := a.do("POST", "/api/links/delete", map[string]any{"email": "alice", "id": added[1].ID}, "admin"); code != 404 {
		t.Errorf("a second delete = %d", code)
	}
	if got := a.custom.store.list("alice"); len(got) != 1 {
		t.Errorf("after delete: %+v", got)
	}
}

func TestAChangeReachesTheSubscriptionAtOnce(t *testing.T) {
	a := newAPIRig(t)
	a.customSeam(a.custom, "subA") // warm the data
	a.do("POST", "/api/links", map[string]any{"email": "alice", "text": "yptun://fresh"}, "admin")
	a.customSeam(a.custom, "subA")
	waitFor(t, func() bool {
		got, _ := a.customSeam(a.custom, "subA")
		return strings.HasSuffix(got, "yptun://fresh")
	})
}

func TestThePageAndTheSeamAreServed(t *testing.T) {
	a := newAPIRig(t)
	req := httptest.NewRequest("GET", "/ui/", nil)
	rec := httptest.NewRecorder()
	a.mux.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") || !strings.Contains(rec.Body.String(), "lucx-ui.js") {
		t.Errorf("page = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, f := range []string{"/ui/app.js", "/ui/style.css", "/ui/lucx-ui.js"} {
		rec := httptest.NewRecorder()
		a.mux.ServeHTTP(rec, httptest.NewRequest("GET", f, nil))
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Errorf("%s = %d", f, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	a.mux.ServeHTTP(rec, httptest.NewRequest("GET", "/ui/../main.go", nil))
	if rec.Code == 200 {
		t.Error("a file outside the page directory was served")
	}
	call := call("subA", b64(vless), nil)
	call.Seam.Mode = "after"
	raw, _ := json.Marshal(call)
	rec = httptest.NewRecorder()
	a.mux.ServeHTTP(rec, httptest.NewRequest("POST", "/_lucx/seam/after", bytes.NewReader(raw)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"action"`) {
		t.Errorf("seam = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAHugeBodyIsRefused(t *testing.T) {
	a := newAPIRig(t)
	huge := `{"email":"alice","text":"` + strings.Repeat("a", 2<<20) + `"}`
	if code, _ := a.do("POST", "/api/links", huge, "admin"); code == 200 {
		t.Error("a 2 MiB body was accepted")
	}
}

func TestOnlyTheNameCanBeChanged(t *testing.T) {
	a := newAPIRig(t)
	added, _, _ := a.custom.store.add("alice", []newLink{{URL: "yptun://1"}})
	if code, _ := a.do("POST", "/api/links/update", map[string]any{"email": "alice", "id": added[0].ID, "name": "Only name"}, "admin"); code != 200 {
		t.Fatalf("update = %d", code)
	}
	if got := a.custom.store.list("alice")[0]; got.Name != "Only name" || !got.Enabled {
		t.Errorf("stored = %+v", got)
	}
	if code, _ := a.do("POST", "/api/links/update", map[string]any{"email": "alice", "id": added[0].ID, "name": "bad\nname"}, "admin"); code != 400 {
		t.Errorf("a bad name = %d", code)
	}
}

func TestSwitchingOrDeletingALinkReachesTheSubscriptionAtOnce(t *testing.T) {
	a := newAPIRig(t)
	added, _, _ := a.custom.store.add("alice", []newLink{{URL: "yptun://x"}, {URL: "yptun://y"}})
	a.customSeam(a.custom, "subA")
	a.do("POST", "/api/links/update", map[string]any{"email": "alice", "id": added[0].ID, "enabled": false}, "admin")
	a.customSeam(a.custom, "subA")
	waitFor(t, func() bool {
		got, _ := a.customSeam(a.custom, "subA")
		return !strings.Contains(got, "yptun://x") && strings.Contains(got, "yptun://y")
	})
	a.do("POST", "/api/links/delete", map[string]any{"email": "alice", "id": added[1].ID}, "admin")
	a.customSeam(a.custom, "subA")
	waitFor(t, func() bool {
		_, reply := a.customSeam(a.custom, "subA")
		return reply.Action == "keep"
	})
}

func TestTheClientSearchEscapesWhatIsTyped(t *testing.T) {
	a := newAPIRig(t)
	a.do("GET", "/api/clients?search="+"a%26pageSize%3D1000", nil, "admin")
	// the fake panel parses the query it receives: a typed "&" must not become a parameter
	a.panel.mu.Lock()
	defer a.panel.mu.Unlock()
	if len(a.panel.lastPaged) == 0 || strings.Contains(strings.TrimPrefix(a.panel.lastPaged, "/clients/list/paged?"), "&pageSize=1000") {
		t.Errorf("the search text became a parameter: %q", a.panel.lastPaged)
	}
}
