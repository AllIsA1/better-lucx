# better-lucx

Improvements for the [LucX-UI](https://github.com/AlexeyLCP/lucx-ui) subscription, as one plugin. Install it from the Plugins page
(it is in the signed catalog) or by pasting this repository's link.

## OpenFlux in the subscription

The panel can build the link of an OpenFlux inbound (copying it from the inbound or from the client's page works), but the query that
picks the inbounds of a subscription lists the protocols by name and `openflux` is not on that list. A client attached to an OpenFlux
inbound therefore never gets its link from the subscription. The plugin adds the link of every OpenFlux inbound the subscription's
clients belong to, if it is not there yet. Nothing is added for a client that is switched off, expired or out of traffic, nor for an
inbound that is switched off or excluded from subscriptions. If the panel itself is fixed one day, the plugin finds the link already
there and adds nothing.

## Own links

The panel lets you add third-party links to a client, but only of the schemes it knows (it refuses `yptun://`, `qwdtt://`,
`openflux://`, `anytls://` and every scheme it has not heard of). This plugin takes any link of any scheme. Open **Better LucX** in the
sidebar, find a client, paste the links (one per line, with an optional name) and they are added to the end of the client's
subscription. A link can be switched off, renamed or deleted; changes show up in the subscription almost at once.

A link must be one line without spaces or control characters (it becomes one line of the subscription); a client can have up to 100.
A client that is switched off, expired or out of traffic gets nothing, as in the panel. The name is added after `#` when the link has
none (not for `vmess://`, which carries its name inside).

The links are kept by the plugin, in its data directory (`links.json`), by the client's email: a client renamed in the panel loses its
links, and when the panel moves to another server, move the plugin's data directory too.

## What it does not do

Only the usual subscription format (`/<sub path>/<subId>`) is changed; JSON and Clash subscriptions and traffic accounting are not
touched. Changes of clients (switched off, expiry) show up within about 30 seconds. Traffic of third-party links cannot be counted:
it does not pass through the panel.

## Safety

It needs `seam:sub` (take part in subscription answers; shown as dangerous), read access to clients and inbounds, and a sidebar entry.
It answers from memory (the panel is read in the background), so a subscription request is not slowed. Whenever it is off, slow or
fails, the panel answers as it always did, and switching it off or on needs no restart. The page talks only to the plugin, and only
the panel's administrator can change links.

The binaries in `bin/` are built with `make build` (static Linux, `amd64` and `arm64`); the panel downloads the repository as it is and
does not compile anything. MIT licensed.
