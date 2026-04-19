package tui

import (
	"sort"
	"strings"
)

type searchResult struct {
	pkg  string
	sym  string // empty for package-only results
	kind string
}

// scoreMatch returns how well query matches text: 4=prefix-of-last-segment, 3=substring,
// 1=subsequence, 0=no match. Case-insensitive.
func scoreMatch(query, text string) int {
	if query == "" {
		return 1
	}
	q := strings.ToLower(query)
	t := strings.ToLower(text)
	if strings.Contains(t, q) {
		if strings.HasPrefix(t, q) {
			return 4
		}
		if i := strings.LastIndex(t, "/"); i >= 0 && strings.HasPrefix(t[i+1:], q) {
			return 4
		}
		return 3
	}
	// subsequence check
	qi := 0
	for _, c := range t {
		if qi < len(q) && rune(q[qi]) == c {
			qi++
		}
	}
	if qi == len(q) {
		return 1
	}
	return 0
}

// filterResults scores and ranks packages and symbols against query, returning up to maxResults.
func filterResults(query string, pkgs []string, syms []symbolEntry) []searchResult {
	const maxResults = 15
	q := strings.TrimSpace(query)

	type scored struct {
		r     searchResult
		score int
	}
	var candidates []scored

	if q == "" {
		for i, p := range pkgs {
			if i >= maxResults {
				break
			}
			candidates = append(candidates, scored{r: searchResult{pkg: p}})
		}
	} else {
		for _, p := range pkgs {
			seg := p
			if i := strings.LastIndex(p, "/"); i >= 0 {
				seg = p[i+1:]
			}
			s := scoreMatch(q, seg)
			if s == 0 {
				s = scoreMatch(q, p)
			}
			if s > 0 {
				candidates = append(candidates, scored{r: searchResult{pkg: p}, score: s * 10})
			}
		}
		for _, se := range syms {
			s := scoreMatch(q, se.sym.Name)
			if s > 0 {
				candidates = append(candidates, scored{
					r:     searchResult{pkg: se.sym.Package, sym: se.sym.Name, kind: se.sym.Kind},
					score: s*10 + 5, // symbols rank slightly above same-tier pkg matches
				})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].r.sym != candidates[j].r.sym {
			return candidates[i].r.sym < candidates[j].r.sym
		}
		return candidates[i].r.pkg < candidates[j].r.pkg
	})

	n := len(candidates)
	if n > maxResults {
		n = maxResults
	}
	out := make([]searchResult, n)
	for i := range out {
		out[i] = candidates[i].r
	}
	return out
}
