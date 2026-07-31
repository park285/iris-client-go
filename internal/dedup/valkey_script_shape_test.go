package dedup

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	luaCallPattern         = regexp.MustCompile(`\bredis\.call\(\s*["']([A-Za-z]+)["']`)
	luaServerHandlePattern = regexp.MustCompile(`\b(?:redis|server)\b`)
)

const luaShapeRationale = "reserveErrorToken reads a server error as proof that SET never ran, which holds only while SET is the " +
	"last step in the body that reaches the server. This gate matches every textual occurrence of the redis and server " +
	"globals and requires each one to begin a redis.call('NAME' site, so redis.pcall, server.call, redis['call'], " +
	"redis.call with a space before the paren, and aliasing the function into a local are all rejected. " +
	"Two things it does not cover: a global name assembled at runtime, such as _G['red'..'is'], evades a textual scan, " +
	"and it reads reserveScriptBody only, so re-pointing the reserveScript binding at a different body is out of its reach"

func TestReserveScriptCallSequenceIsGetThenSet(t *testing.T) {
	t.Parallel()

	calls := luaCallPattern.FindAllStringSubmatchIndex(reserveScriptBody, -1)
	callStarts := make(map[int]struct{}, len(calls))
	for _, call := range calls {
		callStarts[call[0]] = struct{}{}
	}

	for _, handle := range luaServerHandlePattern.FindAllStringIndex(reserveScriptBody, -1) {
		if _, ok := callStarts[handle[0]]; ok {
			continue
		}

		t.Fatalf(
			"reserveScript touches the server API at offset %d (%q) in a form this gate cannot order; %s",
			handle[0], luaSnippetAt(reserveScriptBody, handle[0]), luaShapeRationale,
		)
	}

	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, strings.ToUpper(reserveScriptBody[call[2]:call[3]]))
	}

	want := []string{"GET", "SET"}
	if !slices.Equal(names, want) {
		t.Fatalf("reserveScript call sequence = %v, want %v; %s", names, want, luaShapeRationale)
	}
}

func luaSnippetAt(body string, at int) string {
	return body[at:min(at+40, len(body))]
}
