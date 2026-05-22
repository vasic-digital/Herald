// Package inbound — classify.go: deterministic §32.6 classifier per Wave 6.5.
//
// Classify translates inbound free-text into a typed Classification so the
// Dispatcher can route by Type (fast-path for command-prefixed messages;
// CC dispatch for free-form). Pure function — no I/O, no global state.
//
// Implementation strategy: a table-driven prefix matcher in fixed precedence
// order. First match wins. The prefix table is normalised to lowercase +
// whitespace-trimmed before comparison so "BUG:" / "bug:" / "  Bug:  " all
// map to the same Type. Criticality is extracted separately via three
// regexes — critical / high / low — falling through to "middle" on no
// match. Confidence is 1.0 on prefix hit, 0.0 on natural-language
// fallthrough.
//
// Per the operator-locked decisions in plan T1, the natural-language
// fallthrough yields Type="query" (§32.6 "natural language → query /
// research / info request"). No LLM fallback in this wave — Wave 7 will
// layer a second-pass LLM classifier on top of this deterministic first
// pass, keyed off Confidence == 0.0.
//
// §107 anchor: the function actually inspects the input. Wave 6.5
// mutation gate M1 (tests/test_wave6.5_mutation_meta.sh) plants
// `func Classify(_ string) Classification { return Classification{} }`
// and the table-driven unit test FAILs because every case asserts a
// specific Type/Criticality/Confidence on the return value. A no-op
// classifier cannot pass.
package inbound

import (
	"regexp"
	"strings"
)

// classifyRule maps one or more command prefixes to the §32.6 Type.
// Prefixes are matched case-insensitively after trimming leading
// whitespace. Order matters across rules — first rule whose prefix
// matches wins. Within a single rule, the listed prefixes are tried in
// order (but they should be disjoint by construction).
type classifyRule struct {
	Prefixes []string
	Type     string
}

// classifyRules is the §32.6 command-prefix table. The order is the
// documented row order in spec V3 §32.6 so future readers can diff
// table-against-spec by sight.
var classifyRules = []classifyRule{
	{Prefixes: []string{"bug:", "issue:"}, Type: "bug"},
	{Prefixes: []string{"task:", "implementation:", "impl:"}, Type: "task"},
	{Prefixes: []string{"investigation:", "investigate:"}, Type: "investigation"},
	{Prefixes: []string{"query:", "question:", "q:", "?"}, Type: "query"},
	{Prefixes: []string{"request:"}, Type: "query"}, // §32.6 row — Request maps to query path
	{Prefixes: []string{"help:", "/help"}, Type: "help_command"},
	{Prefixes: []string{"status:", "/status"}, Type: "status_request"},
	{Prefixes: []string{"continue:", "/continue"}, Type: "continuation_request"},
	{Prefixes: []string{"done:", "resolve:"}, Type: "closure"},
	{Prefixes: []string{"reopen:"}, Type: "reopen"},
	{Prefixes: []string{"override:"}, Type: "override"}, // §32.6 operator type-correction
}

// criticalityCritical / criticalityHigh / criticalityLow are word-boundary
// regexes (case-insensitive) that scan the WHOLE input for an explicit
// criticality keyword. Multiple matches are resolved by precedence:
// critical wins over high wins over low. No match → "middle" (default).
var (
	criticalityCritical = regexp.MustCompile(`(?i)\b(critical|urgent|p0|emergency|sev[-_ ]?1)\b`)
	criticalityHigh     = regexp.MustCompile(`(?i)\b(high|important|p1|sev[-_ ]?2)\b`)
	criticalityLow      = regexp.MustCompile(`(?i)\b(low|trivial|p3|p4|sev[-_ ]?[34])\b`)
)

// Classify inspects text and returns the §32.6 Classification.
//
// Defaults on no-match: Type="query", Criticality="middle",
// Confidence=0.0. The dispatcher's CC call still happens for type=query;
// the LLM is the second-pass classifier in Wave 7 (keyed on
// Confidence == 0.0). For deterministic prefix hits, Confidence=1.0 and
// the dispatcher's fast-path skips CC entirely for the command types
// (help_command / status_request / continuation_request / closure /
// reopen / override).
func Classify(text string) Classification {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)

	c := Classification{Type: "query", Criticality: "middle", Confidence: 0.0}

	for _, r := range classifyRules {
		matched := false
		for _, p := range r.Prefixes {
			if strings.HasPrefix(lower, p) {
				c.Type = r.Type
				c.Confidence = 1.0
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}

	switch {
	case criticalityCritical.MatchString(t):
		c.Criticality = "critical"
	case criticalityHigh.MatchString(t):
		c.Criticality = "high"
	case criticalityLow.MatchString(t):
		c.Criticality = "low"
	}

	return c
}
