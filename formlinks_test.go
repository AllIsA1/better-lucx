// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	formRoute = "/clients/:email/externalLinks"
	getRoute  = "/clients/get/:email"
)

func apiCall(mode, method, route, email string, body []byte, response *SeamResponse) SeamCall {
	var c SeamCall
	c.Seam.Engine, c.Seam.Mode, c.Seam.Route = "api", mode, route
	c.Request = SeamRequest{Method: method, Route: route, Params: map[string]string{"email": email}, Body: body, User: "admin"}
	c.Response = response
	return c
}

func formBody(rows ...map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"externalLinks": rows})
	return b
}

func row(value string, extra ...map[string]any) map[string]any {
	r := map[string]any{"kind": "link", "value": value, "remark": "", "enable": true, "expiryTime": 0, "namePrefix": ""}
	for _, e := range extra {
		for k, v := range e {
			r[k] = v
		}
	}
	return r
}

func okResponse() *SeamResponse {
	return &SeamResponse{Status: 200, Body: []byte(`{"success":true,"msg":"saved","obj":null}`)}
}

type formRig struct {
	*rig
	custom *customLinks
	forms  *formLinks
}

func newFormRig(t *testing.T) *formRig {
	r, c := customRig(t)
	return &formRig{rig: r, custom: c, forms: newFormLinks(c)}
}

func (f *formRig) before(email string, body []byte) (SeamReply, error) {
	return f.forms.onSeam(apiCall("before", "POST", formRoute, email, body, nil))
}

func (f *formRig) after(email string, resp *SeamResponse) (SeamReply, error) {
	return f.forms.onSeam(apiCall("after", "POST", formRoute, email, nil, resp))
}

func forwardedRows(t *testing.T, reply SeamReply) []map[string]any {
	t.Helper()
	if reply.Action != "continue" || reply.Request == nil || reply.Request.Body == nil {
		t.Fatalf("reply = %+v, want a changed request body", reply)
	}
	var body struct {
		ExternalLinks []map[string]any `json:"externalLinks"`
	}
	if err := json.Unmarshal(*reply.Request.Body, &body); err != nil {
		t.Fatal(err)
	}
	return body.ExternalLinks
}

func TestLinksTheFormSendsInASchemeThePanelKnowsGoToThePanel(t *testing.T) {
	f := newFormRig(t)
	known := []string{
		"vless://u@h:1#a", "vmess://eyJwcyI6IngifQ==", "trojan://p@h:1", "ss://YWVzOnBhc3M@h:1",
		"hysteria2://p@h:1", "hy2://p@h:1", "wireguard://k@h:1", "wg://k@h:1",
		"qwdtt://config?x=1", "wdtt://config", "csqtt://connect?host=h", "olcrtc://x",
	}
	var rows []map[string]any
	for _, k := range known {
		rows = append(rows, row(k))
	}
	reply, err := f.before("alice", formBody(rows...))
	if err != nil {
		t.Fatal(err)
	}
	if reply.Action != "continue" || reply.Request != nil {
		t.Errorf("a request with nothing for the plugin must go on untouched: %+v", reply)
	}
}

func TestLinksOfOtherSchemesAreTakenOutOfTheRequestAndTheRestGoesOn(t *testing.T) {
	f := newFormRig(t)
	body := []byte(`{"externalLinks":[` +
		`{"kind":"link","value":"vless://u@h:1#keep","remark":"","enable":true,"expiryTime":0,"namePrefix":""},` +
		`{"kind":"link","value":"amneziawg://IyDwn4e18J","remark":"AWG","enable":true,"expiryTime":0,"namePrefix":""},` +
		`{"kind":"subscription","value":"yptun://not-a-url-but-a-subscription-row","remark":"","enable":true,"expiryTime":0,"namePrefix":"p"},` +
		`{"kind":"link","value":"yptun://inbound?v=2","remark":"","enable":false,"expiryTime":0,"namePrefix":""},` +
		`{"kind":"link","value":"VLESS://u@h:1","remark":"","enable":true,"expiryTime":0,"namePrefix":""},` +
		`{"kind":"link","value":"not a link","remark":"","enable":true,"expiryTime":0,"namePrefix":""},` +
		`{"kind":"link","value":"  ","remark":"","enable":true,"expiryTime":0,"namePrefix":""}],"other":1}`)
	reply, err := f.before("alice", body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(*reply.Request.Body, &out); err != nil || string(out["other"]) != "1" {
		t.Fatalf("the rest of the request must survive: %v %s", err, *reply.Request.Body)
	}
	var kept []map[string]any
	_ = json.Unmarshal(out["externalLinks"], &kept)
	var values []string
	for _, k := range kept {
		values = append(values, k["value"].(string))
	}
	want := []string{"vless://u@h:1#keep", "yptun://not-a-url-but-a-subscription-row", "not a link", "  "}
	if strings.Join(values, "|") != strings.Join(want, "|") {
		t.Errorf("forwarded = %q, want %q (the panel judges known schemes, subscriptions and what is no link at all)", values, want)
	}
	if _, ok := reply.Request.Headers["Content-Length"]; ok {
		t.Error("the plugin must not set the length of the body")
	}
}

func TestNothingIsStoredUntilThePanelHasSaidYes(t *testing.T) {
	f := newFormRig(t)
	f.before("alice", formBody(row("amneziawg://IyDwn4e18J", map[string]any{"remark": "AWG"})))
	if got := f.custom.store.list("alice"); len(got) != 0 {
		t.Fatalf("stored before the panel answered: %+v", got)
	}
	reply, err := f.after("alice", okResponse())
	if err != nil || reply.Action != "keep" {
		t.Fatalf("after = %+v, %v", reply, err)
	}
	got := f.custom.store.list("alice")
	if len(got) != 1 || got[0].URL != "amneziawg://IyDwn4e18J" || got[0].Name != "AWG" || !got[0].Enabled {
		t.Errorf("stored = %+v", got)
	}
}

func TestARefusedSaveLeavesTheLinksAsTheyWere(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("alice", []newLink{{URL: "yptun://old"}})
	f.before("alice", formBody(row("yptun://new"), row("vless://a b")))
	refused := &SeamResponse{Status: 200, Body: []byte(`{"success":false,"msg":"Something went wrong (invalid)","obj":null}`)}
	if reply, err := f.after("alice", refused); err != nil || reply.Action != "keep" {
		t.Fatalf("after = %+v, %v", reply, err)
	}
	if got := f.custom.store.list("alice"); len(got) != 1 || got[0].URL != "yptun://old" {
		t.Errorf("a refused save changed the links: %+v", got)
	}
	for _, resp := range []*SeamResponse{{Status: 500, Body: []byte("oops")}, {Status: 200, Body: []byte("not json")}} {
		f.before("alice", formBody(row("yptun://new")))
		f.after("alice", resp)
		if got := f.custom.store.list("alice"); len(got) != 1 || got[0].URL != "yptun://old" {
			t.Errorf("an answer %d changed the links: %+v", resp.Status, got)
		}
	}
}

func TestTheSavedSetReplacesTheOldOneAndKeepsTheIdsOfLinksThatStay(t *testing.T) {
	f := newFormRig(t)
	added, _, _ := f.custom.store.add("alice", []newLink{{URL: "yptun://stays", Name: "S"}, {URL: "yptun://goes"}})
	f.before("alice", formBody(row("yptun://stays", map[string]any{"remark": "Renamed"}), row("qwdtt2://new")))
	f.after("alice", okResponse())
	got := f.custom.store.list("alice")
	if len(got) != 2 || got[0].ID != added[0].ID || got[0].Name != "Renamed" || got[1].URL != "qwdtt2://new" {
		t.Errorf("stored = %+v", got)
	}
}

func TestDeletingEveryLinkInTheFormClearsThePluginsLinksToo(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("alice", []newLink{{URL: "yptun://a"}})
	reply, _ := f.before("alice", formBody(row("vless://u@h:1")))
	if reply.Request != nil {
		t.Fatalf("nothing to take out: %+v", reply)
	}
	f.after("alice", okResponse())
	if got := f.custom.store.list("alice"); len(got) != 0 {
		t.Errorf("the links stayed: %+v", got)
	}
}

func TestAnAfterWithoutABeforeChangesNothing(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("alice", []newLink{{URL: "yptun://a"}})
	f.after("alice", okResponse())
	if got := f.custom.store.list("alice"); len(got) != 1 {
		t.Errorf("stored = %+v", got)
	}
}

func TestOneClientsSaveDoesNotTouchAnothers(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("bob", []newLink{{URL: "yptun://bobs"}})
	f.before("alice", formBody(row("yptun://alices")))
	f.after("bob", okResponse()) // a different client's answer must not commit alice's links
	if len(f.custom.store.list("alice")) != 0 {
		t.Error("alice's links were committed by bob's answer")
	}
	f.after("alice", okResponse())
	if len(f.custom.store.list("alice")) != 1 || len(f.custom.store.list("bob")) != 1 {
		t.Error("the sets of the two clients are mixed")
	}
}

func TestAStagedSaveIsForgottenAfterAWhile(t *testing.T) {
	f := newFormRig(t)
	f.forms.now = func() time.Time { return f.now }
	f.before("alice", formBody(row("yptun://late")))
	f.now = f.now.Add(2 * stagedLifetime)
	f.after("alice", okResponse())
	if got := f.custom.store.list("alice"); len(got) != 0 {
		t.Errorf("a save from long ago was committed: %+v", got)
	}
}

func TestABrokenLinkOfAnUnknownSchemeIsRefusedWithTheFormsOwnKindOfAnswer(t *testing.T) {
	f := newFormRig(t)
	for name, r := range map[string]map[string]any{
		"a space":        row("yptun://a b"),
		"a line break":   row("yptun://a\nopenflux://b"),
		"too long":       row("yptun://" + strings.Repeat("a", maxLinkLength)),
		"a long name":    row("yptun://ok", map[string]any{"remark": strings.Repeat("n", maxNameLength+1)}),
		"a bad name":     row("yptun://ok", map[string]any{"remark": "a\nb"}),
		"a negative end": row("yptun://ok", map[string]any{"expiryTime": -1}),
	} {
		reply, err := f.before("alice", formBody(r))
		if err != nil || reply.Action != "respond" || *reply.Response.Status != 200 {
			t.Errorf("%s: reply = %+v, %v", name, reply, err)
			continue
		}
		var out struct {
			Success bool   `json:"success"`
			Msg     string `json:"msg"`
		}
		if json.Unmarshal(*reply.Response.Body, &out) != nil || out.Success || !strings.Contains(out.Msg, "invalid share link") {
			t.Errorf("%s: the answer %s must look like the panel's own refusal", name, *reply.Response.Body)
		}
	}
	f.after("alice", okResponse())
	if len(f.custom.store.list("alice")) != 0 {
		t.Error("a refused request left something staged")
	}
}

func TestTooManyLinksAreRefused(t *testing.T) {
	f := newFormRig(t)
	var rows []map[string]any
	for i := 0; i < maxLinksPerClient+1; i++ {
		rows = append(rows, row("yptun://l"+strings.Repeat("a", i)))
	}
	reply, _ := f.before("alice", formBody(rows...))
	if reply.Action != "respond" || !strings.Contains(string(*reply.Response.Body), "at most") {
		t.Errorf("reply = %+v", reply)
	}
}

func TestRepeatedLinksInOneSaveCountOnce(t *testing.T) {
	f := newFormRig(t)
	f.before("alice", formBody(row("yptun://same"), row("yptun://same", map[string]any{"remark": "again"})))
	f.after("alice", okResponse())
	if got := f.custom.store.list("alice"); len(got) != 1 {
		t.Errorf("stored = %+v", got)
	}
}

func TestABodyThatIsNotWhatTheFormSendsIsLeftToThePanel(t *testing.T) {
	f := newFormRig(t)
	for _, body := range [][]byte{nil, []byte("not json"), []byte(`{"externalLinks":"x"}`), []byte(`{}`), []byte(`{"externalLinks":null}`)} {
		reply, err := f.before("alice", body)
		if err != nil || reply.Action != "continue" || reply.Request != nil {
			t.Errorf("body %q: %+v, %v", body, reply, err)
		}
	}
}

// ---- what the form reads back

func getResponse(obj string) *SeamResponse {
	return &SeamResponse{Status: 200, Body: []byte(`{"success":true,"msg":"","obj":` + obj + `}`)}
}

func injected(t *testing.T, reply SeamReply) (rows []map[string]any, obj map[string]json.RawMessage) {
	t.Helper()
	if reply.Action != "modify" || reply.Response == nil || reply.Response.Body == nil {
		t.Fatalf("reply = %+v, want a changed answer", reply)
	}
	var env struct {
		Success bool                       `json:"success"`
		Obj     map[string]json.RawMessage `json:"obj"`
	}
	if err := json.Unmarshal(*reply.Response.Body, &env); err != nil || !env.Success {
		t.Fatalf("answer %s: %v", *reply.Response.Body, err)
	}
	if err := json.Unmarshal(env.Obj["externalLinks"], &rows); err != nil {
		t.Fatal(err)
	}
	return rows, env.Obj
}

func TestTheFormIsShownTheLinksThePluginHolds(t *testing.T) {
	f := newFormRig(t)
	end := f.now.Add(48 * time.Hour).UnixMilli()
	f.custom.store.add("alice", []newLink{{URL: "yptun://one", Name: "One"}, {URL: "amneziawg://IyDw"}})
	list := f.custom.store.list("alice")
	f.custom.store.update("alice", list[1].ID, ptr(false), nil)
	f.custom.store.setExpiry("alice", list[0].ID, end)
	resp := getResponse(`{"client":{"email":"alice","id":7},"inboundIds":[1],"externalLinks":[{"id":3,"clientId":7,"kind":"link","value":"vless://u@h:1","remark":"","enable":true,"expiryTime":0}],"usedTraffic":5}`)
	reply, err := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, resp))
	if err != nil {
		t.Fatal(err)
	}
	rows, obj := injected(t, reply)
	if len(rows) != 3 || rows[0]["value"] != "vless://u@h:1" {
		t.Fatalf("rows = %v: the panel's own link must stay first", rows)
	}
	one, off := rows[1], rows[2]
	if one["kind"] != "link" || one["value"] != "yptun://one" || one["remark"] != "One" || one["enable"] != true || one["expiryTime"] != float64(end) || one["clientId"] != float64(7) {
		t.Errorf("row = %v", one)
	}
	if off["enable"] != false || off["value"] != "amneziawg://IyDw" {
		t.Errorf("row = %v", off)
	}
	if string(obj["usedTraffic"]) != "5" || !strings.Contains(string(obj["client"]), `"id":7`) {
		t.Errorf("the rest of the answer was damaged: %v", obj)
	}
	if one["id"] == nil || one["id"] == rows[0]["id"] || one["id"] == off["id"] {
		t.Errorf("the injected rows need ids of their own: %v %v", one["id"], off["id"])
	}
}

func TestAClientWithNoExternalLinksOfItsOwnStillGetsTheFieldFilled(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("alice", []newLink{{URL: "yptun://one"}})
	for _, obj := range []string{`{"client":{"id":1}}`, `{"client":{"id":1},"externalLinks":null}`, `{"client":{"id":1},"externalLinks":[]}`} {
		reply, _ := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, getResponse(obj)))
		if rows, _ := injected(t, reply); len(rows) != 1 || rows[0]["value"] != "yptun://one" {
			t.Errorf("%s: rows = %v", obj, rows)
		}
	}
}

func TestNothingIsAddedWhereThereIsNothingToAdd(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("bob", []newLink{{URL: "yptun://bobs"}})
	f.custom.store.add("carol", []newLink{{URL: "yptun://carols"}})
	for name, resp := range map[string]*SeamResponse{
		"no links held":               getResponse(`{"client":{"id":1},"externalLinks":[]}`),
		"an error":                    {Status: 200, Body: []byte(`{"success":false,"msg":"record not found","obj":null}`)},
		"a server error":              {Status: 500, Body: []byte(`oops`)},
		"not json":                    {Status: 200, Body: []byte(`<html>`)},
		"no object":                   {Status: 200, Body: []byte(`{"success":true,"obj":null}`)},
		"a list":                      {Status: 200, Body: []byte(`{"success":true,"obj":[1,2]}`)},
		"externalLinks is not a list": getResponse(`{"externalLinks":"x"}`),
	} {
		reply, err := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, resp))
		if err != nil || reply.Action != "keep" {
			t.Errorf("%s: %+v, %v", name, reply, err)
		}
	}
	// a client that has links of its own, asked for another one's email, must not see them
	if reply, _ := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, getResponse(`{"client":{"id":1},"externalLinks":[]}`))); reply.Action != "keep" {
		t.Error("bob's links reached alice's form")
	}
}

func TestOnlyTheDeclaredSeamsAreAnswered(t *testing.T) {
	f := newFormRig(t)
	for name, c := range map[string]SeamCall{
		"another route":     apiCall("before", "POST", "/clients/add", "alice", formBody(row("yptun://x")), nil),
		"another mode":      apiCall("replace", "POST", formRoute, "alice", formBody(row("yptun://x")), nil),
		"before on the get": apiCall("before", "GET", getRoute, "alice", nil, nil),
	} {
		if _, err := f.forms.onSeam(c); err == nil {
			t.Errorf("%s: a seam the plugin never declared was answered", name)
		}
	}
	if len(f.forms.staged) != 0 {
		t.Error("an undeclared call staged something")
	}
}

func TestTheSeamEndpointRoutesApiCallsToTheFormModule(t *testing.T) {
	a := newAPIRig(t)
	raw, _ := json.Marshal(apiCall("before", "POST", formRoute, "alice", formBody(row("yptun://x"), row("vless://u@h:1")), nil))
	reply := postSeam(t, a, "before", raw)
	if reply.Action != "continue" || reply.Request == nil {
		t.Fatalf("reply = %+v", reply)
	}
	raw, _ = json.Marshal(apiCall("after", "POST", formRoute, "alice", nil, okResponse()))
	postSeam(t, a, "after", raw)
	waitFor(t, func() bool { return len(a.custom.store.list("alice")) == 1 })
}

func ptr[T any](v T) *T { return &v }

func TestARowWithoutAKindIsALinkAndBlanksAroundItAreIgnored(t *testing.T) {
	f := newFormRig(t)
	reply, _ := f.before("alice", []byte(`{"externalLinks":[{"value":"  yptun://untyped  "},{"kind":"link","value":"\tqwdtt2://tabbed\n"}]}`))
	if rows := forwardedRows(t, reply); len(rows) != 0 {
		t.Fatalf("forwarded = %v", rows)
	}
	f.after("alice", okResponse())
	got := f.custom.store.list("alice")
	if len(got) != 2 || got[0].URL != "yptun://untyped" || got[1].URL != "qwdtt2://tabbed" {
		t.Errorf("stored = %+v", got)
	}
}

func TestWhenEverythingIsTakenTheFormsListGoesOnEmptyNotNull(t *testing.T) {
	f := newFormRig(t)
	reply, _ := f.before("alice", formBody(row("yptun://a")))
	if !strings.Contains(string(*reply.Request.Body), `"externalLinks":[]`) {
		t.Errorf("body = %s", *reply.Request.Body)
	}
}

func TestAMissingOrNullListIsNotAnEmptySet(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("alice", []newLink{{URL: "yptun://keep"}})
	for _, body := range []string{`{"externalLinks":null}`, `{}`, `not json`, ``} {
		f.before("alice", []byte(body))
		f.after("alice", okResponse())
		if got := f.custom.store.list("alice"); len(got) != 1 {
			t.Errorf("body %q cleared the links: %+v", body, got)
		}
	}
	f.before("alice", []byte(`{"externalLinks":[]}`)) // an explicit empty list is "no links"
	f.after("alice", okResponse())
	if got := f.custom.store.list("alice"); len(got) != 0 {
		t.Errorf("an explicit empty list left %+v", got)
	}
}

func TestEnabledIsTheDefaultAndTheEndOfLifeIsKept(t *testing.T) {
	f := newFormRig(t)
	end := f.now.Add(time.Hour).UnixMilli()
	f.before("alice", []byte(`{"externalLinks":[{"kind":"link","value":"yptun://a","expiryTime":`+jsonNumber(end)+`}]}`))
	f.after("alice", okResponse())
	got := f.custom.store.list("alice")
	if len(got) != 1 || !got[0].Enabled || got[0].ExpiryMs != end {
		t.Errorf("stored = %+v", got)
	}
}

func jsonNumber(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestOnlyAnOKAnswerWithSuccessCommits(t *testing.T) {
	f := newFormRig(t)
	f.before("alice", formBody(row("yptun://a")))
	f.after("alice", &SeamResponse{Status: 500, Body: []byte(`{"success":true}`)})
	if len(f.custom.store.list("alice")) != 0 {
		t.Error("an answer with status 500 committed")
	}
}

func TestACommitIsMadeOnceAndShowsInTheSubscriptionAtOnce(t *testing.T) {
	f := newFormRig(t)
	f.customSeam(f.custom, "subA") // warm the data
	f.before("alice", formBody(row("yptun://one")))
	f.after("alice", okResponse())
	f.custom.store.remove("alice", f.custom.store.list("alice")[0].ID)
	f.after("alice", okResponse()) // a second answer must not commit the same save again
	if len(f.custom.store.list("alice")) != 0 {
		t.Error("the save was committed twice")
	}
	f.before("alice", formBody(row("yptun://two")))
	f.after("alice", okResponse())
	f.customSeam(f.custom, "subA")
	waitFor(t, func() bool {
		got, _ := f.customSeam(f.custom, "subA")
		return strings.HasSuffix(got, "yptun://two")
	})
}

func TestARefusedAnswerOfAGetIsNotTouchedEvenWhenItLooksRight(t *testing.T) {
	f := newFormRig(t)
	f.custom.store.add("alice", []newLink{{URL: "yptun://a"}})
	resp := getResponse(`{"client":{"id":1},"externalLinks":[]}`)
	resp.Status = 500
	if reply, _ := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, resp)); reply.Action != "keep" {
		t.Errorf("reply = %+v", reply)
	}
	if reply, _ := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, getResponse(`{"client":{"id":1},"externalLinks":"x"}`))); reply.Action != "keep" {
		t.Errorf("a field that is not a list was overwritten: %+v", reply)
	}
	failed := &SeamResponse{Status: 200, Body: []byte(`{"success":false,"obj":{"client":{"id":1},"externalLinks":[]}}`)}
	if reply, _ := f.forms.onSeam(apiCall("after", "GET", getRoute, "alice", nil, failed)); reply.Action != "keep" {
		t.Errorf("a failed answer was changed: %+v", reply)
	}
}

func TestALinkWhoseEndHasComeIsNotServed(t *testing.T) {
	r, c := customRig(t)
	now := r.now.UnixMilli()
	added, _, _ := c.store.add("alice", []newLink{{URL: "yptun://past"}, {URL: "yptun://now"}, {URL: "yptun://future"}, {URL: "yptun://forever"}})
	c.store.setExpiry("alice", added[0].ID, now-1000)
	c.store.setExpiry("alice", added[1].ID, now)
	c.store.setExpiry("alice", added[2].ID, now+60000)
	got, _ := r.customSeam(c, "subA")
	lines := strings.Split(got, "\n")
	if len(lines) != 3 || lines[1] != "yptun://future" || lines[2] != "yptun://forever" {
		t.Errorf("lines = %v", lines)
	}
}

func TestAReplacedSetLeavesNoEmptyEntryAndSurvivesAFailedWrite(t *testing.T) {
	s, path := newStore(t)
	s.add("gone@example", []newLink{{URL: "yptun://1"}})
	if err := s.replace("gone@example", nil); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "gone@example") {
		t.Errorf("the file still names the client: %s", raw)
	}
	if os.Geteuid() == 0 {
		return
	}
	s.add("a", []newLink{{URL: "yptun://kept"}})
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if err := s.replace("a", []formRow{{URL: "yptun://other", Enabled: true}}); err == nil {
		t.Error("a write that failed was reported as done")
	}
	if got := s.list("a"); len(got) != 1 || got[0].URL != "yptun://kept" {
		t.Errorf("a failed write changed the links: %+v", got)
	}
	if err := s.replace("fresh", []formRow{{URL: "yptun://x", Enabled: true}}); err == nil {
		t.Error("a write that failed was reported as done")
	}
	if len(s.list("fresh")) != 0 {
		t.Error("a failed first write left the client behind")
	}
	added, _, _ := s.add("b", nil)
	_ = added
	if err := s.setExpiry("a", s.list("a")[0].ID, 5); err == nil {
		t.Error("a failed write of the end time was reported as done")
	} else if s.list("a")[0].ExpiryMs != 0 {
		t.Error("a failed write changed the end time")
	}
}
