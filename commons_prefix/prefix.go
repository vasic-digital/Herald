// Package prefix implements the 3-letter project-prefix algorithm from
// spec V3 §8.2.
//
// When a consuming project does NOT define its own workable-item
// prefix (e.g. `HRD-` for Herald itself), Herald derives a deterministic
// 3-letter prefix from the project name. The result lands in
// `.herald/prefix.lock` (TOML) so the mapping is stable across
// machines and regenerations.
//
// No mature Go library generates 3-letter abbreviations from arbitrary
// project names (the only Go prior art Defacto2/releaser/initialism is
// a curated lookup table, not a generator); Herald ships its own.
package prefix

import (
	"hash/fnv"
	"regexp"
	"strings"
	"unicode"
)

// Generate returns a deterministic 3-letter uppercase prefix for the
// given project name. See spec V3 §8.2 for the algorithm + rationale.
//
// Steps:
//
//  1. Normalize: NFKD strip, retain [A-Za-z0-9], split on CamelCase +
//     [-_ /] into tokens.
//  2. Rule A (>=3 tokens): first letter of each of the first three tokens.
//  3. Rule B (2 tokens): first letter of token 1; first letter of token 2;
//     first internal consonant of token 2.
//  4. Rule C (1 token): first letter; first internal consonant; last consonant.
//  5. Uppercase.
//
// Collision resolution lives in the caller — the lock-file logic
// (planned) reads .herald/prefix.lock and rejects duplicates by
// rewriting the third character via fnv1a32 % 26, iterating up to 26
// times, then falling back to HR0..HR9.
func Generate(name string) string {
	tokens := tokenize(name)
	switch {
	case len(tokens) == 0:
		return "HRD" // fall back to Herald's own prefix when the name is empty/garbage
	case len(tokens) >= 3:
		return strings.ToUpper(string([]byte{
			firstASCII(tokens[0]),
			firstASCII(tokens[1]),
			firstASCII(tokens[2]),
		}))
	case len(tokens) == 2:
		return strings.ToUpper(string([]byte{
			firstASCII(tokens[0]),
			firstASCII(tokens[1]),
			firstInternalConsonant(tokens[1]),
		}))
	default: // exactly 1 token
		t := tokens[0]
		return strings.ToUpper(string([]byte{
			firstASCII(t),
			firstInternalConsonant(t),
			lastConsonant(t),
		}))
	}
}

// Resolve combines Generate with a deterministic collision-resolution
// pass against the provided existing prefix→name map. If the generated
// prefix collides with a DIFFERENT name, the third character is
// rewritten using fnv1a32(name) % 26; iterates up to 26 times, then
// falls back to numeric suffix HR0..HR9.
//
// existing is the map currently persisted to .herald/prefix.lock —
// callers MUST treat the return value as authoritative and write it
// back to the lock file (atomic + committed).
func Resolve(name string, existing map[string]string) string {
	base := Generate(name)
	if owner, taken := existing[base]; !taken || owner == name {
		return base
	}
	hash := fnv1a32(name)
	candidate := []byte(base)
	for i := 0; i < 26; i++ {
		c := byte('A' + (uint32(i)+hash)%26)
		candidate[2] = c
		got := string(candidate)
		if owner, taken := existing[got]; !taken || owner == name {
			return got
		}
	}
	for d := byte('0'); d <= byte('9'); d++ {
		candidate[2] = d
		got := string(candidate)
		if owner, taken := existing[got]; !taken || owner == name {
			return got
		}
	}
	// 26 letters + 10 digits exhausted (vanishingly unlikely). Fall back
	// to "HR0" and let the operator pick manually via lock-file edit.
	return "HR0"
}

// --- internals ---------------------------------------------------------

var splitOn = regexp.MustCompile(`[-_ /]+`)

func tokenize(name string) []string {
	// 1. NFKD-strip non-ASCII letters/digits (approximation; full ICU not
	//    required for prefix generation — operators with non-Latin names
	//    can map them manually in the lock file).
	stripped := make([]rune, 0, len(name))
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			stripped = append(stripped, r)
		} else if r == '-' || r == '_' || r == ' ' || r == '/' {
			stripped = append(stripped, r)
		}
	}
	// 2. Split on delimiters.
	parts := splitOn.Split(string(stripped), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		// 3. Split each part on CamelCase boundaries (lower→upper).
		out = append(out, splitCamel(p)...)
	}
	return out
}

func splitCamel(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	prev := rune(0)
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(prev) {
			out = append(out, s[start:i])
			start = i
		}
		prev = r
	}
	out = append(out, s[start:])
	return out
}

func firstASCII(s string) byte {
	for _, r := range s {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			if r >= 'a' && r <= 'z' {
				return byte(r - 32)
			}
			return byte(r)
		}
	}
	return 'X' // intentional: marker for "no first letter"
}

func firstInternalConsonant(s string) byte {
	for i, r := range s {
		if i == 0 {
			continue
		}
		if r >= 'a' && r <= 'z' {
			r -= 32
		}
		if isConsonant(byte(r)) {
			return byte(r)
		}
	}
	return firstASCII(s) // fall back to first letter when no consonant found
}

func lastConsonant(s string) byte {
	upper := strings.ToUpper(s)
	for i := len(upper) - 1; i >= 0; i-- {
		if isConsonant(upper[i]) {
			return upper[i]
		}
	}
	return firstASCII(s)
}

func isConsonant(b byte) bool {
	if b < 'A' || b > 'Z' {
		return false
	}
	switch b {
	case 'A', 'E', 'I', 'O', 'U', 'Y':
		return false
	}
	return true
}

func fnv1a32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
