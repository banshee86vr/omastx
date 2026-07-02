// Package drift is the product's correctness core: semver-aware tag selection and
// drift scoring (SPEC §2.2 tag rules, §2.3 drift model). It is intentionally pure
// (strings in, values out) so it can be table-tested to ≥90% coverage (SPEC §7).
package drift

import (
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Class is the coarse drift bucket surfaced in the UI (SPEC §2.3).
type Class string

const (
	Current    Class = "current"
	Patch      Class = "patch"
	Minor      Class = "minor"
	Major      Class = "major"
	Deprecated Class = "deprecated"
	Unknown    Class = "unknown"
)

// nonComparable tags are recorded but never used for semver comparison (SPEC §2.2).
var nonComparable = map[string]bool{
	"latest":  true,
	"edge":    true,
	"stable":  true,
	"master":  true,
	"main":    true,
	"nightly": true,
}

// Channel identifies the release line a tag belongs to. Only tags in the same
// channel as the installed tag are comparable (SPEC §2.2): same "v" prefix and
// same prerelease family (e.g. installed 1.2.3-alpine → candidates *-alpine).
type Channel struct {
	Prefix string // "v" or ""
	Family string // prerelease stem: "alpine" from "alpine3.19", "rc" from "rc1", "" for stable
}

// Parse attempts to read a tag as a channel-qualified semver version. ok is false
// for non-semver tags (date tags, git shas, "latest", ...): those are never guessed.
func Parse(tag string) (v *semver.Version, ch Channel, ok bool) {
	t := strings.TrimSpace(tag)
	if t == "" || nonComparable[strings.ToLower(t)] {
		return nil, Channel{}, false
	}
	if strings.HasPrefix(t, "v") || strings.HasPrefix(t, "V") {
		ch.Prefix = "v"
	}
	parsed, err := semver.StrictNewVersion(strings.TrimPrefix(strings.TrimPrefix(t, "v"), "V"))
	if err != nil {
		// Fall back to lenient parsing (handles "1.2" etc.) but reject sha/date noise.
		parsed, err = semver.NewVersion(t)
		if err != nil {
			return nil, Channel{}, false
		}
		// semver.NewVersion accepts bare "20240115" as 20240115.0.0 — reject
		// obvious date-only / single-number tags so they stay "unknown".
		if !strings.Contains(t, ".") {
			return nil, Channel{}, false
		}
	}
	ch.Family = family(parsed.Prerelease())
	return parsed, ch, true
}

// family strips trailing version noise from a prerelease to get its stem.
// "alpine3.19" → "alpine", "rc1" → "rc", "beta.1" → "beta", "" → "".
func family(pre string) string {
	if pre == "" {
		return ""
	}
	if i := strings.IndexAny(pre, ".-"); i >= 0 {
		pre = pre[:i]
	}
	pre = strings.TrimRightFunc(pre, func(r rune) bool { return r >= '0' && r <= '9' })
	return pre
}

// Compute classifies installed→latest into a drift class and score (SPEC §2.3).
// drift_score = Δmajor*10000 + Δminor*100 + Δpatch. Non-semver on either side, or
// a latest that is not ahead, yields current/unknown accordingly — never a guess.
func Compute(installed, latest string) (Class, float64) {
	iv, _, iok := Parse(installed)
	if !iok {
		return Unknown, 0
	}
	lv, _, lok := Parse(latest)
	if !lok {
		return Unknown, 0
	}
	if !lv.GreaterThan(iv) {
		return Current, 0
	}
	dMajor := int64(lv.Major()) - int64(iv.Major())
	dMinor := int64(lv.Minor()) - int64(iv.Minor())
	dPatch := int64(lv.Patch()) - int64(iv.Patch())
	score := float64(dMajor*10000 + dMinor*100 + dPatch)
	switch {
	case dMajor > 0:
		return Major, score
	case dMinor > 0:
		return Minor, score
	case dPatch > 0:
		return Patch, score
	default:
		return Current, 0
	}
}

// Selection is the resolver's tag-selection outcome for one artifact.
type Selection struct {
	Latest         string   // best comparable tag in the installed channel ("" if none)
	Candidates     []string // comparable tags in-channel, newest first (includes latest)
	ReleasesBehind int      // count strictly newer than installed, -1 if not comparable
}

// SelectLatest picks the newest tag in the same channel as installed, from the
// available tags. Non-semver and out-of-channel tags are ignored for comparison
// (SPEC §2.2). If installed itself is non-semver, nothing is comparable.
func SelectLatest(installed string, available []string) Selection {
	iv, ich, iok := Parse(installed)
	if !iok {
		return Selection{ReleasesBehind: -1}
	}

	type cand struct {
		raw string
		v   *semver.Version
	}
	var inChannel []cand
	seen := map[string]bool{}
	for _, tag := range available {
		if seen[tag] {
			continue
		}
		seen[tag] = true
		v, ch, ok := Parse(tag)
		if !ok || ch != ich {
			continue
		}
		inChannel = append(inChannel, cand{raw: tag, v: v})
	}
	if len(inChannel) == 0 {
		return Selection{ReleasesBehind: -1}
	}

	sort.SliceStable(inChannel, func(i, j int) bool {
		return inChannel[i].v.GreaterThan(inChannel[j].v)
	})

	candidates := make([]string, 0, len(inChannel))
	behind := 0
	for _, c := range inChannel {
		candidates = append(candidates, c.raw)
		if c.v.GreaterThan(iv) {
			behind++
		}
	}
	return Selection{
		Latest:         inChannel[0].raw,
		Candidates:     candidates,
		ReleasesBehind: behind,
	}
}
