package diffparse

import "unicode/utf8"

// Limits on intra-line refinement. The diff is quadratic in token count, and
// on minified or generated files the result would be noise anyway.
const (
	maxRefineBytes  = 4000
	maxRefineTokens = 800

	// Above this fraction of changed tokens the two lines have little in
	// common, and marking almost all of both is worse than marking neither.
	maxChangedRatio = 0.7

	// maxRefinedPairs caps the work across a whole diff, not just one line.
	// Refinement costs roughly eight times the rest of parsing, so a diff with
	// hundreds of thousands of changed lines would spend most of its parse
	// time marking runs nobody will scroll to. Past the cap the lines still
	// render, just without the intraline marks.
	maxRefinedPairs = 20000
)

// refineHunk finds lines that are two versions of each other and marks the
// parts that actually differ, so a one-character change does not read as a
// whole line rewritten.
func refineHunk(h *Hunk, budget *int) {
	lines := h.Lines
	for i := 0; i < len(lines) && *budget > 0; {
		if lines[i].Kind != Removed {
			i++
			continue
		}
		delStart := i
		for i < len(lines) && lines[i].Kind == Removed {
			i++
		}
		addStart := i
		for i < len(lines) && lines[i].Kind == Added {
			i++
		}
		dels := lines[delStart:addStart]
		adds := lines[addStart:i]
		if len(adds) == 0 {
			continue
		}
		pairRun(dels, adds, budget)
	}
}

// pairRun matches removed lines to added lines within one replaced run.
func pairRun(dels, adds []Line, budget *int) {
	switch {
	case len(dels) == len(adds):
		for i := range dels {
			if *budget <= 0 {
				return
			}
			*budget--
			refinePair(&dels[i], &adds[i])
		}
	case len(dels) == 1:
		*budget--
		refinePair(&dels[0], &adds[bestMatch(dels[0].Text, adds)])
	case len(adds) == 1:
		*budget--
		refinePair(&dels[bestMatch(adds[0].Text, dels)], &adds[0])
	}
	// Runs of differing length with several lines on both sides have no
	// obvious pairing; leaving them unrefined is better than guessing.
}

// bestMatch returns the index of the line in candidates sharing the longest
// common prefix and suffix with text.
func bestMatch(text string, candidates []Line) int {
	best, bestScore := 0, -1
	for i, c := range candidates {
		score := commonPrefix(text, c.Text) + commonSuffix(text, c.Text)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

func refinePair(del, add *Line) {
	if len(del.Text) > maxRefineBytes || len(add.Text) > maxRefineBytes {
		return
	}
	oldToks := tokenize(del.Text)
	newToks := tokenize(add.Text)
	if len(oldToks) > maxRefineTokens || len(newToks) > maxRefineTokens {
		return
	}

	oldChanged, newChanged := tokenDiff(del.Text, oldToks, add.Text, newToks)
	if changedRatio(oldToks, oldChanged) > maxChangedRatio &&
		changedRatio(newToks, newChanged) > maxChangedRatio {
		return
	}
	del.Segments = segments(oldToks, oldChanged, len(del.Text))
	add.Segments = segments(newToks, newChanged, len(add.Text))
}

// changedRatio is the share of a line's non-whitespace tokens that changed.
// Whitespace is excluded from both sides of the fraction so that indentation
// does not make two unrelated lines look related.
func changedRatio(toks []token, changed []bool) float64 {
	total, n := 0, 0
	for i, t := range toks {
		if t.space {
			continue
		}
		total++
		if changed[i] {
			n++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total)
}

// token is one unit of intra-line comparison: a word, a run of whitespace, or
// a single punctuation character.
type token struct {
	start, end int
	space      bool
}

func tokenize(s string) []token {
	var toks []token
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		start := i
		switch {
		case isSpace(r):
			for i < len(s) {
				r, size = utf8.DecodeRuneInString(s[i:])
				if !isSpace(r) {
					break
				}
				i += size
			}
			toks = append(toks, token{start, i, true})
		case isWord(r):
			for i < len(s) {
				r, size = utf8.DecodeRuneInString(s[i:])
				if !isWord(r) {
					break
				}
				i += size
			}
			toks = append(toks, token{start, i, false})
		default:
			i += size
			toks = append(toks, token{start, i, false})
		}
	}
	return toks
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\r' }

func isWord(r rune) bool {
	return r == '_' || r >= utf8.RuneSelf ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// tokenDiff marks which tokens on each side are not part of the longest common
// subsequence of the two token streams.
func tokenDiff(oldStr string, oldToks []token, newStr string, newToks []token) (oldChanged, newChanged []bool) {
	n, m := len(oldToks), len(newToks)
	oldChanged = make([]bool, n)
	newChanged = make([]bool, m)

	// Trim the matching head and tail first: most edits are local, and this
	// keeps the quadratic part small.
	lo := 0
	for lo < n && lo < m && oldStr[oldToks[lo].start:oldToks[lo].end] == newStr[newToks[lo].start:newToks[lo].end] {
		lo++
	}
	hiOld, hiNew := n, m
	for hiOld > lo && hiNew > lo &&
		oldStr[oldToks[hiOld-1].start:oldToks[hiOld-1].end] == newStr[newToks[hiNew-1].start:newToks[hiNew-1].end] {
		hiOld--
		hiNew--
	}

	a, b := oldToks[lo:hiOld], newToks[lo:hiNew]
	// Classic LCS table over the remaining window.
	rows, cols := len(a)+1, len(b)+1
	lcs := make([]int, rows*cols)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if oldStr[a[i].start:a[i].end] == newStr[b[j].start:b[j].end] {
				lcs[i*cols+j] = lcs[(i+1)*cols+j+1] + 1
			} else if lcs[(i+1)*cols+j] >= lcs[i*cols+j+1] {
				lcs[i*cols+j] = lcs[(i+1)*cols+j]
			} else {
				lcs[i*cols+j] = lcs[i*cols+j+1]
			}
		}
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case oldStr[a[i].start:a[i].end] == newStr[b[j].start:b[j].end]:
			i++
			j++
		case lcs[(i+1)*cols+j] >= lcs[i*cols+j+1]:
			oldChanged[lo+i] = true
			i++
		default:
			newChanged[lo+j] = true
			j++
		}
	}
	for ; i < len(a); i++ {
		oldChanged[lo+i] = true
	}
	for ; j < len(b); j++ {
		newChanged[lo+j] = true
	}
	return oldChanged, newChanged
}

// segments collapses per-token flags into byte ranges covering the whole line.
func segments(toks []token, changed []bool, length int) []Segment {
	var segs []Segment
	pos := 0
	add := func(start, end int, ch bool) {
		if start >= end {
			return
		}
		if n := len(segs); n > 0 && segs[n-1].Changed == ch && segs[n-1].End == start {
			segs[n-1].End = end
			return
		}
		segs = append(segs, Segment{start, end, ch})
	}
	for i, t := range toks {
		add(pos, t.start, false)
		// Whitespace between two changed tokens reads better as part of the
		// change than as a gap in it.
		ch := changed[i]
		if t.space && ch {
			ch = i > 0 && changed[i-1] && i+1 < len(toks) && changed[i+1]
		}
		add(t.start, t.end, ch)
		pos = t.end
	}
	add(pos, length, false)

	// A run with nothing marked needs no per-segment rendering at all.
	for _, s := range segs {
		if s.Changed {
			return segs
		}
	}
	return nil
}

func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func commonSuffix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return i
}
