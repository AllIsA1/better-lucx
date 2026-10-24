// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"embed"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// The page is part of the binary, so the plugin needs no file access beyond its own state directory.
//
//go:embed ui
var pages embed.FS

const maxBody = 1 << 20

// newMux serves everything the plugin answers: the subscription seam, the page and the calls the page makes.
// The panel's page bridge only reaches /api/..., which is where the page's calls go.
func newMux(api panelAPI, f *fixer, c *customLinks) http.Handler {
	s := &server{api: api, links: c}
	inner := http.NewServeMux()
	inner.HandleFunc("GET /overview", s.overview)
	inner.HandleFunc("GET /clients", s.clients)
	inner.HandleFunc("GET /links", s.list)
	inner.HandleFunc("POST /links", s.add)
	inner.HandleFunc("POST /links/update", s.update)
	inner.HandleFunc("POST /links/delete", s.remove)

	mux := http.NewServeMux()
	forms := newFormLinks(c)
	mux.Handle("POST /_lucx/seam/{mode}", SeamHandler(func(call SeamCall) (SeamReply, error) {
		if call.Seam.Engine == "api" {
			return forms.onSeam(call)
		}
		return subscriptionSeam(call, awgFormat{}, f, c)
	}))
	mux.Handle("/api/", http.StripPrefix("/api", inner))
	mux.Handle("/ui/", pageHandler())
	return mux
}

type server struct {
	api   panelAPI
	links *customLinks
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// admin lets a call through only when the panel says an administrator made it.
func admin(w http.ResponseWriter, r *http.Request) bool {
	if RequestUser(r) == "" {
		fail(w, http.StatusForbidden, "only the panel's administrator may change links")
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "the request is not valid JSON of the expected shape")
		return false
	}
	return true
}

func (s *server) overview(w http.ResponseWriter, _ *http.Request) {
	rows := s.links.store.overview()
	if rows == nil {
		rows = []overviewRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"clients": rows})
}

type clientRow struct {
	Email  string `json:"email"`
	SubID  string `json:"subId"`
	Enable bool   `json:"enable"`
	Links  int    `json:"links"`
}

// clients searches the panel's clients (for the page's picker) and says how many links each has.
func (s *server) clients(w http.ResponseWriter, r *http.Request) {
	var page struct {
		Items []struct {
			Email  string `json:"email"`
			SubID  string `json:"subId"`
			Enable bool   `json:"enable"`
		} `json:"items"`
	}
	path := "/clients/list/paged?pageSize=20&page=1&search=" + url.QueryEscape(strings.TrimSpace(r.URL.Query().Get("search")))
	if err := s.api.Do(r.Context(), http.MethodGet, path, nil, &page); err != nil {
		fail(w, http.StatusBadGateway, "the panel did not answer: "+err.Error())
		return
	}
	out := []clientRow{}
	for _, c := range page.Items {
		out = append(out, clientRow{Email: c.Email, SubID: c.SubID, Enable: c.Enable, Links: len(s.links.store.list(c.Email))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"clients": out})
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		fail(w, http.StatusBadRequest, "email is required")
		return
	}
	links := s.links.store.list(email)
	if links == nil {
		links = []storedLink{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "links": links})
}

func (s *server) add(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var body struct {
		Email string    `json:"email"`
		Text  string    `json:"text"`
		Name  string    `json:"name"`
		Links []newLink `json:"links"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" {
		fail(w, http.StatusBadRequest, "email is required")
		return
	}
	var known clientRecord
	if err := s.api.Do(r.Context(), http.MethodGet, clientPath(email), nil, &known); err != nil {
		fail(w, http.StatusNotFound, "no such client (or the panel did not answer): "+err.Error())
		return
	}
	batch := append([]newLink(nil), body.Links...)
	for _, line := range strings.Split(body.Text, "\n") {
		if strings.TrimSpace(line) != "" {
			batch = append(batch, newLink{URL: line, Name: body.Name})
		}
	}
	added, rejected, err := s.links.store.add(email, batch)
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot save the links: "+err.Error())
		return
	}
	s.links.invalidate()
	if added == nil {
		added = []storedLink{}
	}
	if rejected == nil {
		rejected = []rejection{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "rejected": rejected})
}

func (s *server) update(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var body struct {
		Email   string  `json:"email"`
		ID      string  `json:"id"`
		Enabled *bool   `json:"enabled"`
		Name    *string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	s.finish(w, s.links.store.update(body.Email, body.ID, body.Enabled, body.Name))
}

func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	if !admin(w, r) {
		return
	}
	var body struct {
		Email string `json:"email"`
		ID    string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	s.finish(w, s.links.store.remove(body.Email, body.ID))
}

func (s *server) finish(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoLink):
		fail(w, http.StatusNotFound, err.Error())
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
	default:
		s.links.invalidate()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// pageHandler answers GET /ui/... with the embedded files; a path ending in / serves its index.html.
func pageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		clean := path.Clean(name)
		if !strings.HasPrefix(clean, "ui/") {
			http.NotFound(w, r)
			return
		}
		data, err := pages.ReadFile(clean)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		typ := mime.TypeByExtension(path.Ext(clean))
		if typ == "" {
			typ = "application/octet-stream"
		}
		w.Header().Set("Content-Type", typ)
		_, _ = w.Write(data)
	})
}
