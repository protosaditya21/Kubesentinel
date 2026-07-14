// Package filters implements the tier-1 and tier-2 checks described in the
// architecture doc (\u00A74.1): cheap, synchronous, in-process filtering that
// runs on every request before anything expensive gets involved.
package filters

import "regexp"

// RegexFilter is the tier-1, sub-millisecond pre-filter: known injection
// markers, denylisted patterns, structural anomalies. It is deliberately
// dumb and fast — it exists to catch the obvious cases so the tier-2
// classifier only has to look at what's left.
type RegexFilter struct {
	patterns []*regexp.Regexp
}

// DefaultPatterns is a starter set. Extend this with patterns specific to
// your threat model — this list is intentionally not exhaustive.
func DefaultPatterns() []string {
	return []string{
		`(?i)ignore (all )?(previous|prior|above) instructions`,
		`(?i)you are now (in )?(developer|debug|admin) mode`,
		`(?i)disregard (the )?system prompt`,
		`(?i)reveal (your|the) (system prompt|instructions)`,
		`[A-Za-z0-9+/]{200,}={0,2}`, // long base64-looking blobs
	}
}

func NewRegexFilter(patterns []string) (*RegexFilter, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, re)
	}
	return &RegexFilter{patterns: compiled}, nil
}

// Verdict is shared across tier-1 and tier-2 checks.
type Verdict struct {
	Blocked bool
	Reason  string
	// Score is in [0,1]; tier-1 only ever returns 0 or 1, tier-2 can be graded.
	Score float64
}

func (f *RegexFilter) Check(input string) Verdict {
	for _, re := range f.patterns {
		if re.MatchString(input) {
			return Verdict{Blocked: true, Reason: "matched pattern: " + re.String(), Score: 1.0}
		}
	}
	return Verdict{Blocked: false, Score: 0.0}
}
