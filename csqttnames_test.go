// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func csqttInbound(port int, remark string) map[string]any {
	return map[string]any{"id": port, "protocol": "csqtt", "remark": remark, "port": port, "enable": true}
}

func csqttLine(peer string) string {
	return "csqtt://connect?host=203.0.113.5&password=pw&peer=" + peer + "&v=2&hashes=aa+bb"
}

func TestNamesACsqttLineAfterItsInbound(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = append(r.panel.inbounds, csqttInbound(46000, "Польша CSQTT"))
	names := newCsqttNames(r.panel)
	names.firstLoadWait = 2 * 1e9
	got := names.apply("sub", []string{"vless://x#a", csqttLine("46000")})
	want := csqttLine("46000") + "#%D0%9F%D0%BE%D0%BB%D1%8C%D1%88%D0%B0%20CSQTT"
	if got[0] != "vless://x#a" || got[1] != want {
		t.Errorf("lines = %q, want the csqtt line to end with the escaped remark", got)
	}
}

func TestCsqttNameFollowsThePeerPortAmongSeveralInbounds(t *testing.T) {
	rows := []string{"46000\tOne", "46010\tTwo"}
	if got := csqttName(csqttLine("46010"), rows); got != "Two" {
		t.Errorf("name = %q, want Two", got)
	}
	if got := csqttName(csqttLine("9"), rows); got != "" {
		t.Errorf("name = %q, a line that fits none of several inbounds must stay as it is", got)
	}
	if got := csqttName(csqttLine("9"), rows[:1]); got != "One" {
		t.Errorf("name = %q, the only inbound names a line whatever its port", got)
	}
}

func TestACsqttLineThatHasANameIsLeftAlone(t *testing.T) {
	r := newRig(t)
	r.panel.inbounds = append(r.panel.inbounds, csqttInbound(46000, "Poland"))
	names := newCsqttNames(r.panel)
	names.firstLoadWait = 2 * 1e9
	line := csqttLine("46000") + "#Mine"
	if got := names.apply("sub", []string{line}); got[0] != line {
		t.Errorf("line = %q", got[0])
	}
	if r.panel.count("/inbounds/list/slim") != 0 {
		t.Error("a subscription without an unnamed csqtt line must not make the plugin ask the panel")
	}
}

func TestSubscriptionWithoutCsqttIsUntouched(t *testing.T) {
	r := newRig(t)
	names := newCsqttNames(r.panel)
	in := []string{"vless://x#a", strings.Repeat("a", 3)}
	if got := names.apply("sub", in); len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Errorf("lines = %q", got)
	}
}

// The panel from lucx.310 on gives the line as csqtt://<base64url sealed payload>: no query to read the port from.
func TestNamesAnEncryptedCsqttLineAfterTheOnlyInbound(t *testing.T) {
	line := "csqtt://AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123-_AbCdEf0123"
	if !isUnnamedCsqtt(line) {
		t.Fatal("an encrypted line without a name was not recognised")
	}
	if got := csqttName(line, []string{"46000\tPoland"}); got != "Poland" {
		t.Errorf("name = %q, want the only inbound's remark", got)
	}
	if got := csqttName(line, []string{"46000\tOne", "46010\tTwo"}); got != "" {
		t.Errorf("name = %q: with several inbounds an encrypted line cannot be told apart", got)
	}
	r := newRig(t)
	r.panel.inbounds = append(r.panel.inbounds, csqttInbound(46000, "Польша CSQTT"))
	names := newCsqttNames(r.panel)
	names.firstLoadWait = 2 * 1e9
	if got := names.apply("sub", []string{line}); got[0] != line+"#%D0%9F%D0%BE%D0%BB%D1%8C%D1%88%D0%B0%20CSQTT" {
		t.Errorf("line = %q", got[0])
	}
}
