// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import "net/http"

// newMux serves the subscription seam, the only thing the plugin answers. The plugin has no page.
func newMux(api panelAPI, f *openfluxNames) http.Handler {
	names := newCsqttNames(api)
	mux := http.NewServeMux()
	mux.Handle("POST /_lucx/seam/{mode}", SeamHandler(func(call SeamCall) (SeamReply, error) {
		return subscriptionSeam(call, awgFormat{}, f, names, csqttPlain{})
	}))
	return mux
}
