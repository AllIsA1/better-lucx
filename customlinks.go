// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

// The "own links" module: any link, of any scheme, can be given to a client and reaches the client's
// subscription. The panel itself refuses a link whose scheme it does not know.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxLinkLength     = 8 << 10
	maxNameLength     = 100
	maxLinksPerClient = 100
)

var linkPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://[^\s\x00-\x1f\x7f]+$`)

// validateLink returns the link trimmed, or says why it cannot be a line of a subscription. Any scheme is
// fine; what is not fine is anything that could add a line or break one.
func validateLink(raw string) (string, error) {
	link := strings.TrimSpace(raw)
	switch {
	case link == "":
		return "", errors.New("empty")
	case len(link) > maxLinkLength:
		return "", fmt.Errorf("longer than %d characters", maxLinkLength)
	case !linkPattern.MatchString(link):
		return "", errors.New("not a link of the form scheme://… without spaces or line breaks")
	}
	return link, nil
}

func validateName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if utf8.RuneCountInString(name) > maxNameLength {
		return "", fmt.Errorf("a name is at most %d characters", maxNameLength)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("a name has no line breaks or control characters")
		}
	}
	return name, nil
}

// applyName gives a link a visible name: the part after #, which clients show. A link that has one, and a
// vmess:// link (its name lives inside it), keep theirs.
func applyName(link, name string) string {
	if name == "" || strings.Contains(link, "#") || strings.HasPrefix(strings.ToLower(link), "vmess://") {
		return link
	}
	return link + "#" + url.PathEscape(name)
}

type storedLink struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
}

type newLink struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type rejection struct {
	Line   string `json:"line"`
	Reason string `json:"reason"`
}

type overviewRow struct {
	Email string `json:"email"`
	Links int    `json:"links"`
}

// linkStore holds the links of every client, by the client's email, in one file of the plugin's data directory.
type linkStore struct {
	mu      sync.Mutex
	path    string
	clients map[string][]storedLink
}

type storeFile struct {
	Version int                     `json:"version"`
	Clients map[string][]storedLink `json:"clients"`
}

// openLinkStore reads the file. A file that cannot be read is kept as a backup beside it and the store starts
// empty: losing the links is bad, but a plugin that cannot start is worse, and the backup can be put back.
func openLinkStore(path string) (*linkStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	s := &linkStore{path: path, clients: map[string][]storedLink{}}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(raw, &f); err != nil {
		backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
		if werr := os.WriteFile(backup, raw, 0o600); werr != nil {
			return nil, fmt.Errorf("the links file is damaged (%v) and cannot be saved aside: %w", err, werr)
		}
		log.Printf("better-lucx: the links file is damaged (%v); kept as %s, starting empty", err, backup)
		return s, nil
	}
	if f.Clients != nil {
		s.clients = f.Clients
	}
	return s, nil
}

func (s *linkStore) saveLocked() error {
	raw, err := json.MarshalIndent(storeFile{Version: 1, Clients: s.clients}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".links-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *linkStore) list(email string) []storedLink {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]storedLink(nil), s.clients[email]...)
}

func (s *linkStore) emails() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.clients))
	for email, links := range s.clients {
		if len(links) > 0 {
			out = append(out, email)
		}
	}
	sort.Strings(out)
	return out
}

func (s *linkStore) overview() []overviewRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []overviewRow
	for email, links := range s.clients {
		if len(links) > 0 {
			out = append(out, overviewRow{Email: email, Links: len(links)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}

// add stores the good links and reports the others with the reason. The error is for a file that cannot be written.
func (s *linkStore) add(email string, links []newLink) (added []storedLink, rejected []rejection, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	have := s.clients[email]
	known := map[string]bool{}
	for _, l := range have {
		known[l.URL] = true
	}
	for _, in := range links {
		link, verr := validateLink(in.URL)
		if verr == nil {
			var name string
			if name, verr = validateName(in.Name); verr == nil {
				switch {
				case known[link]:
					verr = errors.New("already added")
				case len(have) >= maxLinksPerClient:
					verr = fmt.Errorf("a client has at most %d links", maxLinksPerClient)
				default:
					l := storedLink{ID: newID(), URL: link, Name: name, Enabled: true}
					have = append(have, l)
					known[link] = true
					added = append(added, l)
				}
			}
		}
		if verr != nil {
			rejected = append(rejected, rejection{Line: in.URL, Reason: verr.Error()})
		}
	}
	if len(added) == 0 {
		return nil, rejected, nil
	}
	s.clients[email] = have
	if err := s.saveLocked(); err != nil {
		s.clients[email] = have[:len(have)-len(added)]
		return nil, nil, err
	}
	return added, rejected, nil
}

var errNoLink = errors.New("no such link")

// update changes what is given (nil leaves a field alone).
func (s *linkStore) update(email, id string, enabled *bool, name *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	links := s.clients[email]
	for i := range links {
		if links[i].ID != id {
			continue
		}
		old := links[i]
		if enabled != nil {
			links[i].Enabled = *enabled
		}
		if name != nil {
			n, err := validateName(*name)
			if err != nil {
				return err
			}
			links[i].Name = n
		}
		if err := s.saveLocked(); err != nil {
			links[i] = old
			return err
		}
		return nil
	}
	return errNoLink
}

func (s *linkStore) remove(email, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	links := s.clients[email]
	for i := range links {
		if links[i].ID != id {
			continue
		}
		rest := append(append([]storedLink(nil), links[:i]...), links[i+1:]...)
		if len(rest) == 0 {
			delete(s.clients, email)
		} else {
			s.clients[email] = rest
		}
		if err := s.saveLocked(); err != nil {
			s.clients[email] = links
			return err
		}
		return nil
	}
	return errNoLink
}

// customLinks gives the subscription of each client the links stored for it.
type customLinks struct {
	*cachedMap
	store *linkStore
	api   panelAPI
}

func newCustomLinks(api panelAPI, store *linkStore) *customLinks {
	c := &customLinks{store: store, api: api}
	c.cachedMap = newCachedMap("own links", c.loadLinks)
	return c
}

type clientRecord struct {
	Client struct {
		SubID      string `json:"subId"`
		Enable     bool   `json:"enable"`
		TotalGB    int64  `json:"totalGB"`
		ExpiryTime int64  `json:"expiryTime"`
	} `json:"client"`
	UsedTraffic int64 `json:"usedTraffic"`
}

func clientPath(email string) string { return "/clients/get/" + url.PathEscape(email) }

// loadLinks asks the panel for each client that has links where its subscription is and whether it may be
// served. A client the panel does not know gives nothing; if the panel answers for none of them, that is a
// failure, so the data known so far stays.
func (c *customLinks) loadLinks(ctx context.Context) (map[string][]string, error) {
	emails := c.store.emails()
	data := map[string][]string{}
	var firstErr error
	failed := 0
	nowMs := c.now().UnixMilli()
	for _, email := range emails {
		var rec clientRecord
		if err := c.api.Do(ctx, http.MethodGet, clientPath(email), nil, &rec); err != nil {
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", email, err)
			}
			continue
		}
		if rec.Client.SubID == "" || !usableRecord(rec.Client.Enable, rec.Client.ExpiryTime, rec.Client.TotalGB, rec.UsedTraffic, nowMs) {
			continue
		}
		for _, l := range c.store.list(email) {
			if l.Enabled {
				data[rec.Client.SubID] = appendNew(data[rec.Client.SubID], []string{applyName(l.URL, l.Name)})
			}
		}
	}
	if len(emails) > 0 && failed == len(emails) {
		return nil, firstErr
	}
	return data, nil
}
