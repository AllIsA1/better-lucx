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

## Links of any scheme in the client form

The client form of the panel (Clients → edit → Links) refuses every link whose scheme the panel does not know: `amneziawg://`, `yptun://`,
`tuic://`, … With this plugin the form takes them. When the form is saved the plugin lifts the links of unknown schemes out of the request,
lets the panel have the rest, and keeps those links itself (after the panel has said yes); when the form reads the client back the plugin
puts them into the list again. They are written into the client's subscription like the links added on the Better LucX page, with the
remark as the name after `#`, switched off or ended links are not served. A link of a scheme the panel knows stays the panel's: it is
checked by the panel and gets its own error message. A link that could damage the subscription (a space, a line break) is refused with
the panel's usual kind of message. To read the client the plugin needs the right to take part in the answers of the clients API
(`seam:api:clients`, shown as dangerous: the client's data passes through it; it only adds its own links).

## AmneziaWG link format

For an AmneziaWG node the panel hands a client `vpn://…` (and, for the AWG inbounds, the older `amneziawg://key@host?…` line); only the
INCY client gets `amneziawg://<base64url config>#name`. The plugin adds that last line, the same config as in `vpn://`, right after every
`vpn://` line of the subscription, for every client. A line that is already there is not added again.

## AmneziaWG on the subscription page

The panel's subscription page (Configs tab) has the format switch itself, from panel build `-plugins.7` on: one row per AmneziaWG node
with `vpn://`, `amneziawg://` (the config), the older `amneziawg://` link and `.conf`. The plugin only provides the `amneziawg://<config>`
line the switch offers; it does not change the page.

## CSQTT name

The panel's `csqtt://` line has no name, so proxy apps show "csqtt [host]". The plugin appends the inbound's remark as the name
(`#<remark>`), choosing the inbound by the line's `peer` port (or the only CSQTT inbound there is). The sealed `csqtt://<base64>` lines of newer panels carry no readable port, so they are named after the only CSQTT inbound. A line that already has a name is left alone.

## What it does not do

Only the usual subscription format (`/<sub path>/<subId>`) is changed (the page a browser shows is left alone); JSON and Clash subscriptions and traffic accounting are not
touched. Changes of clients (switched off, expiry) show up within about 30 seconds. Traffic of third-party links cannot be counted:
it does not pass through the panel.

## Safety

It needs `seam:sub` (take part in subscription answers; shown as dangerous), read access to clients and inbounds, and a sidebar entry.
It answers from memory (the panel is read in the background), so a subscription request is not slowed. Whenever it is off, slow or
fails, the panel answers as it always did, and switching it off or on needs no restart. The page talks only to the plugin, and only
the panel's administrator can change links.

The binaries in `bin/` are built with `make build` (static Linux, `amd64` and `arm64`); the panel downloads the repository as it is and
does not compile anything. MIT licensed.
