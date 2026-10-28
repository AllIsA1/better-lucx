# better-lucx

Subscription extras for [LucX-UI](https://github.com/AlexeyLCP/lucx-ui) that the panel does not do itself, as one plugin. Install it from the Plugins page
(it is in the signed catalog) or by pasting this repository's link.

## What the panel does itself now (removed in 2.0.0)

These were parts of this plugin; the panel took them over, so they are gone from it:

- **OpenFlux in the subscription**: the panel does it since lucx.313 (the plugin stopped adding them in 1.4.3).
- **Links of any scheme for a client**: since lucx.323 ([#145](https://github.com/AlexeyLCP/lucx-ui/issues/145)) the client form of the panel
  (Clients → edit → Links) takes any `scheme://…` link, so the plugin's own link store, its links-in-the-form seams and its page are removed.
  **Links that were kept only by the plugin** (its `links.json` in the plugin's data directory, never deleted by the plugin) are no longer
  written into the subscriptions: add them again in the client form. Older plugin versions' links are in the file for reference.
- **A name for the olcRTC / CSQTT / qWDTT / OpenFlux line**: since lucx.324 ([#136](https://github.com/AlexeyLCP/lucx-ui/issues/136)) a stored
  link, or an inbound's line of those protocols, may contain `$name`, which the panel turns into the name; the panel also no longer rewrites
  the `#fragment` of such links. Put `$name` in the free-form part of the link where there is one (olcRTC: the meet URL). The sealed CSQTT and
  OpenFlux links of the panel have no place for it, so the page names below stay.

## AmneziaWG link format

For an AmneziaWG node the panel hands a client `vpn://…` (and, for the AWG inbounds, the older `amneziawg://key@host?…` line); only the
INCY client gets `amneziawg://<base64url config>#name`. The plugin adds that last line, the same config as in `vpn://`, right after every
`vpn://` line of the subscription, for every client. A line that is already there is not added again.

## AmneziaWG on the subscription page

The panel's subscription page (Configs tab) has the format switch itself, from panel build `-plugins.7` on: one row per AmneziaWG node
with `vpn://`, `amneziawg://` (the config), the older `amneziawg://` link and `.conf`. The plugin only provides the `amneziawg://<config>`
line the switch offers; it does not change the page.

## CSQTT and OpenFlux names on the page

The panel names no CSQTT or OpenFlux line, so the subscription page would call them "Link N". The plugin tells the page what to call
them (the remark of their inbound) in a separate list next to the links. **The lines themselves are never changed**: the CSQTT app
reads everything after `csqtt://` as the sealed payload, so a `#name` made it drop the link (better-lucx 1.3.1 and 1.3.2 did add one;
1.4.1 does not). A proxy client shows such a profile under the name it makes up itself.

## Plain CSQTT links in the subscription

The panel (lucx.310 on) gives a CSQTT inbound as a sealed link, `csqtt://<base64url>`. The proxy clients that fetch a subscription
(YPtun, for one) read only the plain form, `csqtt://connect?v=2&host=…&peer=…&password=…&hashes=a+b`, and skip the sealed one. The plugin
opens each sealed CSQTT line of the raw subscription and sends the plain form of the same link (the person's own password, no `#name`)
to every client. The page a browser shows is left as the panel made it, and so are the links the panel hands out for copying: the
official CSQTT app never fetches a subscription, a person pastes a link into it, and it **accepts the sealed link only**.

## The subscription page

What the plugin adds to the raw subscription (the `amneziawg://` config line) is added to the
list of links on the page a browser shows for the subscription too, so the page and the proxy client agree. The plugin changes only
the list the panel embeds in the page (`window.__SUB_PAGE_DATA__`); a custom page theme that does not carry it is left as it is.

## What it does not do

Only the usual subscription format (`/<sub path>/<subId>`) and its page are changed; JSON and Clash subscriptions and traffic accounting are not
touched. The inbounds the plugin reads (for the CSQTT and OpenFlux names) are refreshed about every 30 seconds.

## Safety

It needs `seam:sub` (take part in subscription answers; shown as dangerous) and read access to inbounds. It has no page and no sidebar
entry. It answers from memory (the panel is read in the background), so a subscription request is not slowed. Whenever it is off, slow or
fails, the panel answers as it always did, and switching it off or on needs no restart.

The binaries in `bin/` are built with `make build` (static Linux, `amd64` and `arm64`); the panel downloads the repository as it is and
does not compile anything. MIT licensed.
