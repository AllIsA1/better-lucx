# better-lucx

Improvements for the [LucX-UI](https://github.com/AlexeyLCP/lucx-ui) subscription, as one plugin. Install it from the Plugins page
(it is in the signed catalog) or by pasting this repository's link.

## OpenFlux in the subscription

Since lucx.313 the panel puts the OpenFlux links into the subscription itself, so the plugin no longer does (better-lucx up to 1.4.2
added them for older panels; it does not any more). It only tells the subscription page what to call them, see below. A panel older
than lucx.313 needs better-lucx 1.4.2.

## Own links

Links of any scheme that were added to clients with this plugin keep working: they are written into the client's subscription as
before. The plugin has no sidebar page any more (better-lucx 1.4.3): the panel build `…-plugins.6` and later takes a link of any scheme in
the client form itself (Clients → edit → Links), without any plugin, so that is where new links go. Links the plugin already holds are
shown in the client form too (see below) and can be changed there.

A link must be one line without spaces or control characters (it becomes one line of the subscription); a client can have up to 100.
A client that is switched off, expired or out of traffic gets nothing, as in the panel. The name is added after `#` when the link has
none (not for `vmess://`, which carries its name inside).

The links are kept by the plugin, in its data directory (`links.json`), by the client's email: a client renamed in the panel loses its
links, and when the panel moves to another server, move the plugin's data directory too.

## Links of any scheme in the client form

The client form of the panel (Clients → edit → Links) refuses every link whose scheme the panel does not know: `amneziawg://`, `yptun://`,
`tuic://`, … With this plugin the form takes them. When the form is saved the plugin lifts the links of unknown schemes out of the request,
lets the panel have the rest, and keeps those links itself (after the panel has said yes); when the form reads the client back the plugin
puts them into the list again. They are written into the client's subscription like the links the plugin holds, with the
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

What the plugin adds to the raw subscription (own links, the `amneziawg://` config line, CSQTT names) is added to the
list of links on the page a browser shows for the subscription too, so the page and the proxy client agree. The plugin changes only
the list the panel embeds in the page (`window.__SUB_PAGE_DATA__`); a custom page theme that does not carry it is left as it is.

## What it does not do

Only the usual subscription format (`/<sub path>/<subId>`) and its page are changed; JSON and Clash subscriptions and traffic accounting are not
touched. Changes of clients (switched off, expiry) show up within about 30 seconds. Traffic of third-party links cannot be counted:
it does not pass through the panel.

## Safety

It needs `seam:sub` (take part in subscription answers; shown as dangerous), read access to clients and inbounds, and a sidebar entry.
It answers from memory (the panel is read in the background), so a subscription request is not slowed. Whenever it is off, slow or
fails, the panel answers as it always did, and switching it off or on needs no restart. The page talks only to the plugin, and only
the panel's administrator can change links.

The binaries in `bin/` are built with `make build` (static Linux, `amd64` and `arm64`); the panel downloads the repository as it is and
does not compile anything. MIT licensed.
