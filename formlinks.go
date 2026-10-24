// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "links in the client form" module: the client form of the panel takes a link of any scheme. The panel
// refuses every scheme it does not know, so those links are taken out of the form's request, kept by the
// plugin (which puts them into the subscription), and shown to the form again when it reads the client back.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// panelSchemes are the link schemes the panel's own external link check accepts (ParseLink in the panel);
// a link starting with one of them stays the panel's business, down to its error message.
var panelSchemes = []string{
	"vmess://", "vless://", "trojan://", "ss://", "hysteria2://", "hy2://", "wireguard://", "wg://",
	"qwdtt://", "wdtt://", "csqtt://", "olcrtc://",
}

var schemePrefix = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://`)

// stagedLifetime: how long the plugin waits for the panel's answer to a save before it forgets what it took.
const stagedLifetime = 30 * time.Second

// formRow is one external link of the form, as far as the plugin keeps it.
type formRow struct {
	URL      string
	Name     string
	Enabled  bool
	ExpiryMs int64
}

type stagedSave struct {
	rows []formRow
	at   time.Time
}

type formLinks struct {
	custom *customLinks
	now    func() time.Time

	mu     sync.Mutex
	staged map[string]stagedSave // by client email, from the request to the answer
}

func newFormLinks(c *customLinks) *formLinks {
	return &formLinks{custom: c, now: time.Now, staged: map[string]stagedSave{}}
}

// onSeam answers the three seams of this module; any other call is a mistake of the caller.
func (f *formLinks) onSeam(c SeamCall) (SeamReply, error) {
	email := c.Request.Params["email"]
	switch {
	case c.Seam.Mode == "before" && c.Request.Method == http.MethodPost && c.Seam.Route == "/clients/:email/externalLinks":
		return f.take(email, c.Request.Body), nil
	case c.Seam.Mode == "after" && c.Request.Method == http.MethodPost && c.Seam.Route == "/clients/:email/externalLinks":
		f.commit(email, c.Response)
		return Keep(), nil
	case c.Seam.Mode == "after" && c.Request.Method == http.MethodGet && c.Seam.Route == "/clients/get/:email":
		return f.show(email, c.Response), nil
	}
	return SeamReply{}, fmt.Errorf("no seam api/%s/%s %s is declared", c.Seam.Mode, c.Request.Method, c.Seam.Route)
}

type formInput struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Remark     string `json:"remark"`
	Enable     *bool  `json:"enable"`
	ExpiryTime int64  `json:"expiryTime"`
}

// take takes the links the panel would refuse out of the request and keeps them aside until the panel has
// answered. Everything else goes on to the panel unchanged.
func (f *formLinks) take(email string, body []byte) SeamReply {
	var req map[string]json.RawMessage
	if json.Unmarshal(body, &req) != nil {
		return Continue(nil)
	}
	var rawRows []json.RawMessage
	if json.Unmarshal(req["externalLinks"], &rawRows) != nil || rawRows == nil {
		return Continue(nil)
	}
	var kept []json.RawMessage
	var taken []formRow
	for _, raw := range rawRows {
		var in formInput
		value := ""
		if json.Unmarshal(raw, &in) == nil {
			value = strings.TrimSpace(in.Value)
		}
		if (in.Kind != "link" && in.Kind != "") || !schemePrefix.MatchString(value) || hasPanelScheme(value) {
			kept = append(kept, raw)
			continue
		}
		row, err := checkedRow(in)
		if err != nil {
			return refusal(err, value)
		}
		taken = append(taken, row)
	}
	if len(distinctRows(taken)) > maxLinksPerClient {
		return refusal(fmt.Errorf("a client has at most %d links", maxLinksPerClient), "")
	}
	f.stage(email, taken)
	if len(taken) == 0 {
		return Continue(nil)
	}
	if kept == nil {
		kept = []json.RawMessage{}
	}
	req["externalLinks"], _ = json.Marshal(kept)
	out, err := json.Marshal(req)
	if err != nil {
		return Continue(nil)
	}
	return Continue(&RequestChange{Body: Bytes(out)})
}

func hasPanelScheme(value string) bool {
	for _, p := range panelSchemes {
		if strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}

func distinctRows(rows []formRow) map[string]bool {
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.URL] = true
	}
	return seen
}

func checkedRow(in formInput) (formRow, error) {
	url, err := validateLink(in.Value)
	if err != nil {
		return formRow{}, err
	}
	name, err := validateName(in.Remark)
	if err != nil {
		return formRow{}, err
	}
	if in.ExpiryTime < 0 {
		return formRow{}, fmt.Errorf("the end of a link's life must be 0 (never) or a time in milliseconds")
	}
	return formRow{URL: url, Name: name, Enabled: in.Enable == nil || *in.Enable, ExpiryMs: in.ExpiryTime}, nil
}

// refusal answers the form the way the panel answers a link it does not take.
func refusal(err error, value string) SeamReply {
	msg := fmt.Sprintf("Something went wrong (invalid share link (%s): %s)", err, value)
	body, _ := json.Marshal(map[string]any{"success": false, "msg": msg, "obj": nil})
	return Respond(http.StatusOK, map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}, body)
}

func (f *formLinks) stage(email string, rows []formRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	for k, s := range f.staged {
		if now.Sub(s.at) > stagedLifetime {
			delete(f.staged, k)
		}
	}
	f.staged[email] = stagedSave{rows: rows, at: now}
}

// commit stores what was taken out of a save, if the panel accepted the save.
func (f *formLinks) commit(email string, resp *SeamResponse) {
	f.mu.Lock()
	s, ok := f.staged[email]
	delete(f.staged, email)
	fresh := ok && f.now().Sub(s.at) <= stagedLifetime
	f.mu.Unlock()
	if !fresh || resp == nil || resp.Status != http.StatusOK {
		return
	}
	var env struct {
		Success bool `json:"success"`
	}
	if json.Unmarshal(resp.Body, &env) != nil || !env.Success {
		return
	}
	if err := f.custom.store.replace(email, s.rows); err != nil {
		return
	}
	f.custom.invalidate()
}

// show adds the links the plugin holds to the external links the form reads.
func (f *formLinks) show(email string, resp *SeamResponse) SeamReply {
	if resp == nil || resp.Status != http.StatusOK {
		return Keep()
	}
	held := f.custom.store.list(email)
	if len(held) == 0 {
		return Keep()
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(resp.Body, &env) != nil {
		return Keep()
	}
	var ok bool
	if json.Unmarshal(env["success"], &ok) != nil || !ok {
		return Keep()
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(env["obj"], &obj) != nil || obj == nil {
		return Keep()
	}
	var existing []json.RawMessage
	if raw, has := obj["externalLinks"]; has && string(raw) != "null" {
		if json.Unmarshal(raw, &existing) != nil {
			return Keep()
		}
	}
	var client struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(obj["client"], &client)
	rows := append([]json.RawMessage(nil), existing...)
	for i, l := range held {
		raw, _ := json.Marshal(map[string]any{
			"id": -(i + 1), "clientId": client.ID, "kind": "link", "value": l.URL, "remark": l.Name, "enable": l.Enabled,
			"expiryTime": l.ExpiryMs, "namePrefix": "", "lastFetchAt": 0, "lastFetchError": "", "sortIndex": len(existing) + i,
		})
		rows = append(rows, raw)
	}
	obj["externalLinks"], _ = json.Marshal(rows)
	env["obj"], _ = json.Marshal(obj)
	out, err := json.Marshal(env)
	if err != nil {
		return Keep()
	}
	return Modify(ResponseChange{Body: Bytes(out)})
}
