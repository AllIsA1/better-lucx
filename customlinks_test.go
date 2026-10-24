// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnyLinkOfAnySchemeIsAccepted(t *testing.T) {
	for _, link := range []string{
		"vless://u@host.example:443?type=tcp#one",
		"openflux://v1/XM1BioMwFIf",
		"yptun://inbound?v=2&c=eNqV-_",
		"qwdtt://config?hashes=AaLB%2CV4_y&name=%F0%9F%8F%B3&port=9000",
		"amneziawg://IyDwn4e18J-HsSDQ",
		"tuic://uuid:pass@host.example:443?congestion_control=bbr#n",
		"anytls://pass@host.example:443/?sni=x",
		"future+scheme://whatever/at/all",
		"HTTPS://example.com/sub",
	} {
		if got, err := validateLink(link); err != nil || got != link {
			t.Errorf("validateLink(%q) = %q, %v", link, got, err)
		}
	}
}

func TestALinkThatCouldBreakTheSubscriptionIsRefused(t *testing.T) {
	long := "vless://" + strings.Repeat("a", maxLinkLength)
	for name, link := range map[string]string{
		"empty":                 "",
		"blanks only":           "   ",
		"no scheme":             "example.com/path",
		"no host part":          "vless://",
		"a space inside":        "vless://a b",
		"a second line":         "vless://a\nopenflux://b",
		"a carriage return":     "vless://a\rb",
		"a tab":                 "vless://a\tb",
		"a control character":   "vless://a\x07b",
		"a null":                "vless://a\x00b",
		"scheme with a space":   "my scheme://x",
		"scheme starts a digit": "1vless://x",
		"too long":              long,
		"not a link at all":     "hello world",
	} {
		if got, err := validateLink(link); err == nil {
			t.Errorf("%s: %q was accepted as %q", name, link, got)
		}
	}
}

func TestSurroundingBlanksAreTrimmed(t *testing.T) {
	if got, err := validateLink("  vless://a#b \n"); err != nil || got != "vless://a#b" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestANameBecomesTheLinksFragmentOnlyWhereItCan(t *testing.T) {
	cases := []struct{ link, name, want string }{
		{"vless://a@h:1", "My node", "vless://a@h:1#My%20node"},
		{"vless://a@h:1", "", "vless://a@h:1"},
		{"vless://a@h:1#own", "My node", "vless://a@h:1#own"},               // the link has a name already
		{"vmess://eyJwcyI6IngifQ==", "My node", "vmess://eyJwcyI6IngifQ=="}, // the name is inside the link
		{"yptun://inbound?v=2", "Яндекс", "yptun://inbound?v=2#%D0%AF%D0%BD%D0%B4%D0%B5%D0%BA%D1%81"},
	}
	for _, c := range cases {
		if got := applyName(c.link, c.name); got != c.want {
			t.Errorf("applyName(%q, %q) = %q, want %q", c.link, c.name, got, c.want)
		}
	}
}

func TestANameIsCheckedToo(t *testing.T) {
	if _, err := validateName("a\nb"); err == nil {
		t.Error("a name with a line break was accepted")
	}
	if _, err := validateName(strings.Repeat("x", maxNameLength+1)); err == nil {
		t.Error("a very long name was accepted")
	}
	if got, err := validateName("  Poland | VK  "); err != nil || got != "Poland | VK" {
		t.Errorf("got %q, %v", got, err)
	}
}

func newStore(t *testing.T) (*linkStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state", "links.json")
	s, err := openLinkStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestLinksSurviveARestart(t *testing.T) {
	s, path := newStore(t)
	added, rejected, err := s.add("alice@example", []newLink{{URL: "yptun://a", Name: "A"}, {URL: "qwdtt://b"}})
	if err != nil || len(added) != 2 || len(rejected) != 0 {
		t.Fatalf("add = %+v %+v %v", added, rejected, err)
	}
	again, err := openLinkStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got := again.list("alice@example")
	if len(got) != 2 || got[0].URL != "yptun://a" || got[0].Name != "A" || !got[0].Enabled || got[0].ID == "" || got[0].ID == got[1].ID {
		t.Errorf("after a restart: %+v", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file must be private: %v %v", info, err)
	}
}

func TestBadLinksAreRejectedOneByOneAndTheGoodOnesKept(t *testing.T) {
	s, _ := newStore(t)
	added, rejected, err := s.add("a", []newLink{{URL: "yptun://ok"}, {URL: "no scheme"}, {URL: "yptun://ok"}, {URL: "vless://a\nb"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].URL != "yptun://ok" {
		t.Errorf("added = %+v", added)
	}
	if len(rejected) != 3 || rejected[1].Reason != "already added" {
		t.Errorf("rejected = %+v", rejected)
	}
}

func TestAClientHasAtMostSoManyLinks(t *testing.T) {
	s, _ := newStore(t)
	var batch []newLink
	for i := 0; i < maxLinksPerClient+5; i++ {
		batch = append(batch, newLink{URL: "x-scheme://l" + strings.Repeat("a", i%7) + string(rune('a'+i%26)) + strings.Repeat("b", i)})
	}
	added, rejected, err := s.add("a", batch)
	if err != nil || len(added) != maxLinksPerClient || len(rejected) != 5 || !strings.Contains(rejected[0].Reason, "at most") {
		t.Fatalf("added %d, rejected %d (%+v), err %v", len(added), len(rejected), rejected, err)
	}
}

func TestEnableRenameDelete(t *testing.T) {
	s, _ := newStore(t)
	added, _, _ := s.add("a", []newLink{{URL: "yptun://1"}, {URL: "yptun://2"}})
	off := false
	name := "Second"
	if err := s.update("a", added[1].ID, &off, &name); err != nil {
		t.Fatal(err)
	}
	got := s.list("a")
	if got[1].Enabled || got[1].Name != "Second" || !got[0].Enabled {
		t.Errorf("after update: %+v", got)
	}
	if err := s.update("a", "nope", &off, nil); err == nil {
		t.Error("an unknown link was updated")
	}
	if err := s.remove("a", added[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := s.list("a"); len(got) != 1 || got[0].ID != added[1].ID {
		t.Errorf("after delete: %+v", got)
	}
	if err := s.remove("a", "nope"); err == nil {
		t.Error("an unknown link was deleted")
	}
	s.remove("a", added[1].ID)
	if emails := s.emails(); len(emails) != 0 {
		t.Errorf("a client without links is still listed: %v", emails)
	}
}

func TestOverviewCountsLinksPerClient(t *testing.T) {
	s, _ := newStore(t)
	s.add("bob", []newLink{{URL: "yptun://1"}})
	s.add("alice", []newLink{{URL: "yptun://2"}, {URL: "yptun://3"}})
	got := s.overview()
	if len(got) != 2 || got[0].Email != "alice" || got[0].Links != 2 || got[1].Email != "bob" || got[1].Links != 1 {
		t.Errorf("overview = %+v", got)
	}
}

func TestACorruptFileIsKeptAsABackupAndTheStoreStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "links.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := openLinkStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.emails()) != 0 {
		t.Error("a corrupt file produced links")
	}
	backups, _ := filepath.Glob(path + ".corrupt-*")
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if raw, _ := os.ReadFile(backups[0]); string(raw) != "{not json" {
		t.Error("the backup is not the original file")
	}
	if _, _, err := s.add("a", []newLink{{URL: "yptun://1"}}); err != nil {
		t.Errorf("the store cannot be used after a corrupt start: %v", err)
	}
}

// ---- the module: which links go into which subscription

func customRig(t *testing.T) (*rig, *customLinks) {
	r := newRig(t)
	store, _ := newStore(t)
	r.panel.byEmail = map[string]map[string]any{
		"alice": {"client": map[string]any{"email": "alice", "subId": "subA", "enable": true, "totalGB": 0, "expiryTime": 0}, "usedTraffic": 5},
		"bob":   {"client": map[string]any{"email": "bob", "subId": "subB", "enable": true, "totalGB": 0, "expiryTime": 0}, "usedTraffic": 0},
	}
	c := newCustomLinks(r.panel, store)
	c.now = func() time.Time { return r.now }
	c.firstLoadWait = 2 * time.Second
	return r, c
}

func (r *rig) customSeam(c *customLinks, sub string) (string, SeamReply) {
	reply, err := subscriptionSeam(call(sub, b64(vless), nil), c)
	if err != nil {
		r.t.Fatal(err)
	}
	if reply.Action != "modify" {
		return "", reply
	}
	return r.decoded(reply), reply
}

func TestACustomLinkReachesTheSubscriptionOfItsClient(t *testing.T) {
	r, c := customRig(t)
	c.store.add("alice", []newLink{{URL: "yptun://one", Name: "Yandex"}, {URL: "qwdtt://two"}})
	got, _ := r.customSeam(c, "subA")
	if want := vless + "\nyptun://one#Yandex\nqwdtt://two"; got != want {
		t.Errorf("subscription = %q, want %q", got, want)
	}
	if got, reply := r.customSeam(c, "subB"); reply.Action != "keep" {
		t.Errorf("another client's subscription changed: %q", got)
	}
}

func TestSeveralClientsShareOneSubscription(t *testing.T) {
	r, c := customRig(t)
	r.panel.byEmail["bob"]["client"].(map[string]any)["subId"] = "subA"
	c.store.add("alice", []newLink{{URL: "yptun://a"}})
	c.store.add("bob", []newLink{{URL: "yptun://b"}})
	got, _ := r.customSeam(c, "subA")
	if want := vless + "\nyptun://a\nyptun://b"; got != want {
		t.Errorf("subscription = %q", got)
	}
}

func TestSwitchedOffLinksAndClientsGiveNothing(t *testing.T) {
	cases := map[string]func(r *rig, c *customLinks){
		"link switched off": func(r *rig, c *customLinks) {
			off := false
			c.store.update("alice", c.store.list("alice")[0].ID, &off, nil)
		},
		"client switched off": func(r *rig, c *customLinks) {
			r.panel.byEmail["alice"]["client"].(map[string]any)["enable"] = false
		},
		"client expires this very millisecond": func(r *rig, c *customLinks) {
			r.panel.byEmail["alice"]["client"].(map[string]any)["expiryTime"] = r.now.UnixMilli()
		},
		"client expired": func(r *rig, c *customLinks) {
			r.panel.byEmail["alice"]["client"].(map[string]any)["expiryTime"] = r.now.Add(-time.Hour).UnixMilli()
		},
		"client's traffic used up": func(r *rig, c *customLinks) {
			r.panel.byEmail["alice"]["client"].(map[string]any)["totalGB"] = 5
		},
		"client deleted from the panel": func(r *rig, c *customLinks) { delete(r.panel.byEmail, "alice") },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			r, c := customRig(t)
			c.store.add("alice", []newLink{{URL: "yptun://one"}})
			setup(r, c)
			if got, reply := r.customSeam(c, "subA"); reply.Action != "keep" {
				t.Errorf("a link was served: %q", got)
			}
		})
	}
}

func TestEmailsWithOddCharactersAreAskedForSafely(t *testing.T) {
	r, c := customRig(t)
	email := "a b/c+d@x.org?e=1"
	r.panel.byEmail[email] = map[string]any{"client": map[string]any{"email": email, "subId": "subX", "enable": true}, "usedTraffic": 0}
	c.store.add(email, []newLink{{URL: "yptun://odd"}})
	if got, _ := r.customSeam(c, "subX"); !strings.HasSuffix(got, "yptun://odd") {
		t.Errorf("subscription = %q", got)
	}
}

func TestAChangeShowsUpAtOnceAfterInvalidate(t *testing.T) {
	r, c := customRig(t)
	c.store.add("alice", []newLink{{URL: "yptun://one"}})
	r.customSeam(c, "subA")
	c.store.add("alice", []newLink{{URL: "yptun://two"}})
	c.invalidate()
	r.customSeam(c, "subA") // starts the refresh, answered from memory
	waitFor(t, func() bool {
		got, _ := r.customSeam(c, "subA")
		return strings.Contains(got, "yptun://two")
	})
}

func TestWithoutAnyCustomLinkThePanelIsNotAsked(t *testing.T) {
	r, c := customRig(t)
	r.customSeam(c, "subA")
	if n := r.panel.count("/clients/get/alice"); n != 0 {
		t.Errorf("the panel was asked %d times", n)
	}
}

func TestAPanelThatFailsEverywhereLeavesTheOldDataInPlace(t *testing.T) {
	r, c := customRig(t)
	c.store.add("alice", []newLink{{URL: "yptun://one"}})
	r.customSeam(c, "subA")
	r.panel.mu.Lock()
	r.panel.byEmail = map[string]map[string]any{} // every lookup fails
	r.panel.mu.Unlock()
	c.invalidate()
	r.customSeam(c, "subA")
	time.Sleep(100 * time.Millisecond)
	// a failed load keeps what was known; "no such client" for every client is read as a panel problem
	got, _ := r.customSeam(c, "subA")
	if !strings.Contains(got, "yptun://one") {
		t.Errorf("the links vanished when the panel failed: %q", got)
	}
}

func TestBothModulesAddTheirLinksOnce(t *testing.T) {
	r, c := customRig(t)
	r.panel.clients = []map[string]any{client("alice", "subA", []int{1})}
	c.store.add("alice", []newLink{{URL: "yptun://c"}, {URL: linkA}}) // the second one the OpenFlux module adds too
	reply, err := subscriptionSeam(call("subA", b64(vless), nil), r.fix, c)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.decoded(reply), vless+"\n"+linkA+"\nyptun://c"; got != want {
		t.Errorf("subscription = %q, want %q", got, want)
	}
}

func TestAnEmptyLinkIsSaidToBeEmpty(t *testing.T) {
	if _, err := validateLink("  "); err == nil || err.Error() != "empty" {
		t.Errorf("err = %v", err)
	}
}

func TestTheDataDirectoryIsPrivate(t *testing.T) {
	_, path := newStore(t)
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the state directory: %v %v", info, err)
	}
}

func TestAClientWithoutLinksLeavesNoTraceInTheFile(t *testing.T) {
	s, path := newStore(t)
	added, _, _ := s.add("gone@example", []newLink{{URL: "yptun://1"}})
	s.remove("gone@example", added[0].ID)
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "gone@example") {
		t.Errorf("the file still names the client: %s", raw)
	}
}

func TestEmailsComeInOrder(t *testing.T) {
	s, _ := newStore(t)
	for _, e := range []string{"f", "b", "d", "a", "e", "c"} {
		s.add(e, []newLink{{URL: "yptun://" + e}})
	}
	for i := 0; i < 5; i++ {
		if got := strings.Join(s.emails(), ""); got != "abcdef" {
			t.Fatalf("emails = %q", got)
		}
	}
}

func TestAFileThatCannotBeWrittenLeavesTheStoreAsItWas(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	s, path := newStore(t)
	added, _, _ := s.add("a", []newLink{{URL: "yptun://1"}})
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if _, _, err := s.add("a", []newLink{{URL: "yptun://2"}}); err == nil {
		t.Error("a write that failed was reported as done")
	}
	if got := s.list("a"); len(got) != 1 {
		t.Errorf("a failed add left %+v", got)
	}
	off := false
	if err := s.update("a", added[0].ID, &off, nil); err == nil {
		t.Error("an update that could not be saved was reported as done")
	}
	if got := s.list("a"); !got[0].Enabled {
		t.Error("a failed update changed the link")
	}
	if err := s.remove("a", added[0].ID); err == nil {
		t.Error("a delete that could not be saved was reported as done")
	}
	if got := s.list("a"); len(got) != 1 {
		t.Error("a failed delete removed the link")
	}
}
