// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log"
	"path/filepath"
)

func main() {
	var api panelAPI
	if p, err := NewPanel(); err != nil {
		log.Printf("better-lucx: %v; subscriptions are left as they are", err)
		api = failingPanel{err}
	} else {
		api = p
	}
	store, err := openLinkStore(filepath.Join(StateDir(), "links.json"))
	if err != nil {
		log.Fatalf("better-lucx: %v", err)
	}
	log.Print("better-lucx plugin ready")
	if err := Serve(newMux(api, newOpenfluxNames(api), newCustomLinks(api, store))); err != nil {
		log.Fatal(err)
	}
}

type panelAPI interface {
	Do(ctx context.Context, method, path string, in, out any) error
}

type failingPanel struct{ err error }

func (f failingPanel) Do(context.Context, string, string, any, any) error { return f.err }
