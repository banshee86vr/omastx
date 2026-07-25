// Package match implements nova-style Artifact Hub package matching heuristics
// (SPEC §2.2): score candidates by name, repo/home URL, description, maintainers.
package match

import (
	"net/url"
	"strings"
)

// Package is the subset of an Artifact Hub search hit used for scoring.
type Package struct {
	Name          string
	Normalized    string
	Repository    string // repository display name
	RepositoryURL string
	HomeURL       string
	Description   string
	Maintainers   []string
}

// Input carries the installed chart metadata we match against.
type Input struct {
	ChartName   string
	ChartRepo   string
	Home        string
	Description string
	Maintainers []string
}

// Score returns a 0–1 confidence that hubPkg is the upstream for the installed chart.
func Score(in Input, hubPkg Package) float32 {
	var score float32

	name := strings.ToLower(strings.TrimSpace(in.ChartName))
	if name != "" {
		pn := strings.ToLower(hubPkg.Normalized)
		if pn == "" {
			pn = strings.ToLower(hubPkg.Name)
		}
		if pn == name {
			score += 0.4
		}
	}

	if in.ChartRepo != "" && hubPkg.RepositoryURL != "" {
		if urlsMatch(in.ChartRepo, hubPkg.RepositoryURL) {
			score += 0.3
		}
	}

	if in.Home != "" && hubPkg.HomeURL != "" {
		if urlsMatch(in.Home, hubPkg.HomeURL) {
			score += 0.15
		}
	}

	if in.Description != "" && hubPkg.Description != "" {
		a := strings.ToLower(strings.TrimSpace(in.Description))
		b := strings.ToLower(strings.TrimSpace(hubPkg.Description))
		if a == b || strings.Contains(b, a) || strings.Contains(a, b) {
			score += 0.1
		}
	}

	if len(in.Maintainers) > 0 && len(hubPkg.Maintainers) > 0 {
		if maintainerOverlap(in.Maintainers, hubPkg.Maintainers) {
			score += 0.15
		}
	}

	if score > 1 {
		return 1
	}
	return score
}

// Best picks the highest-scoring package; ok is false when nothing scores above zero.
func Best(in Input, pkgs []Package) (pkg Package, confidence float32, ok bool) {
	for _, p := range pkgs {
		s := Score(in, p)
		if s > confidence {
			confidence = s
			pkg = p
			ok = true
		}
	}
	return pkg, confidence, ok
}

// LowConfidenceThreshold is the cutoff below which the UI shows "unverified match".
const LowConfidenceThreshold float32 = 0.6

func maintainerOverlap(a, b []string) bool {
	set := map[string]bool{}
	for _, m := range a {
		set[strings.ToLower(strings.TrimSpace(m))] = true
	}
	for _, m := range b {
		if set[strings.ToLower(strings.TrimSpace(m))] {
			return true
		}
	}
	return false
}

func urlsMatch(a, b string) bool {
	na, err := normalizeURL(a)
	if err != nil {
		return false
	}
	nb, err := normalizeURL(b)
	if err != nil {
		return false
	}
	return na == nb
}

func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimSuffix(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
	u.Fragment = ""
	u.RawQuery = ""
	return u.String(), nil
}
