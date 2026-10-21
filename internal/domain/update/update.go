// Package update decides whether a newer goozels release than the running
// build exists and words the notice. It is pure: the release lookup is an
// adapter (adapter/releasecheck) injected by the caller.
package update

import (
	"strconv"
	"strings"
)

// Command is how users upgrade goozels; the notice points at it.
const Command = "go install gooze.dev/pkg/goozels/cmd/goozels@latest"

// Notice returns the one-line message announcing latest to a user running
// current, and false when latest is not newer (or either is unparsable, as for
// "(devel)" builds).
func Notice(current, latest string) (string, bool) {
	if !IsNewer(latest, current) {
		return "", false
	}

	return "goozels " + latest + " is available (you have " + current + "). Update: " + Command, true
}

// Released reports whether v is a release or pseudo-version, rather than an
// unknown or "(devel)" build that cannot be compared.
func Released(v string) bool {
	_, ok := parse(v)

	return ok
}

// IsNewer reports whether candidate is a higher semantic version than current.
// Both are "vMAJOR.MINOR.PATCH" with an optional pre-release and build suffix;
// anything unparsable is never newer.
func IsNewer(candidate, current string) bool {
	c, ok := parse(candidate)
	if !ok {
		return false
	}

	cur, ok := parse(current)
	if !ok {
		return false
	}

	return compare(c, cur) > 0
}

type version struct {
	core [3]int
	pre  string
}

func parse(v string) (version, bool) {
	rest, ok := strings.CutPrefix(v, "v")
	if !ok {
		return version{}, false
	}

	rest, _, _ = strings.Cut(rest, "+")
	rest, pre, _ := strings.Cut(rest, "-")

	parts := strings.Split(rest, ".")
	if len(parts) != len(version{}.core) {
		return version{}, false
	}

	var out version

	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return version{}, false
		}

		out.core[i] = n
	}

	out.pre = pre

	return out, true
}

// compare orders by the numeric core, then ranks a release above any of its
// pre-releases, so a pseudo-version like v0.2.1-0.20260101-abcdef (a commit
// after v0.2.0) sorts above v0.2.0 and below v0.2.1. Pre-releases compare
// lexically, which is enough to rank them against the latest release tag.
func compare(a, b version) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			if a.core[i] > b.core[i] {
				return 1
			}

			return -1
		}
	}

	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	}

	return strings.Compare(a.pre, b.pre)
}
