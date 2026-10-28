// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log"
)

func main() {
	var api panelAPI
	if p, err := NewPanel(); err != nil {
		log.Printf("better-lucx: %v; subscriptions are left as they are", err)
		api = failingPanel{err}
	} else {
		api = p
	}
	log.Print("better-lucx plugin ready")
	if err := Serve(newMux(api, newOpenfluxNames(api))); err != nil {
		log.Fatal(err)
	}
}

type panelAPI interface {
	Do(ctx context.Context, method, path string, in, out any) error
}

type failingPanel struct{ err error }

func (f failingPanel) Do(context.Context, string, string, any, any) error { return f.err }
