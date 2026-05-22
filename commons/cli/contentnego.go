// Wave 4b Task 3 — Accept-header content negotiation primitive.
//
// Pure functions over header strings + a typed media-type lookup. No Gin
// coupling at this layer; the Gin glue lives in commons/cli/toon.go
// (W4b-T4) which calls NegotiateContentType + MarshalChosen +
// UnmarshalChosen below.
//
// Policy (per W4b operator decisions 1–6, plan §"Operator-locked
// decisions" 2026-05-22):
//
//   - Accept: */*                 ⇒ server default (Herald default = TOON;
//                                     operator may flip via the env var
//                                     HERALD_DEFAULT_RESPONSE_CODEC=json)
//   - Accept: application/toon    ⇒ TOON
//   - Accept: application/json    ⇒ JSON
//   - Accept: both with no q      ⇒ TOON wins (Herald-default tie-break)
//   - Accept: both with q params  ⇒ higher q wins
//   - Accept: missing / empty     ⇒ server default
//   - Content-Type: application/toon          ⇒ TOON
//   - Content-Type: application/json or empty ⇒ JSON (backwards-compat:
//                                                curl -d '...' usage)
//
// §107 anti-bluff anchors enforced at this layer:
//
//   - MarshalChosen with ct="application/toon" goes through the REAL
//     digital.vasic.toon (which wraps github.com/toon-format/toon-go since
//     W4b-T2). The output never begins with '{' or '[' — the original
//     2026-05-17 PASS-bluff would have surfaced as JSON-syntax bytes here.
//   - UnmarshalChosen round-trips the same bytes back into the same Go
//     struct; "no error" alone is insufficient evidence per Universal
//     §11.4 / Herald §107. The paired test asserts reflect.DeepEqual.
//   - UnmarshalChosen with an unknown content-type returns an explicit
//     error naming the unsupported CT — never a silent JSON fallback that
//     would mask a misrouted body.
//
// Constitutional anchors: Universal §11.4 (PASS-bluff prevention),
// Herald §107 (end-user-usability covenant), W4b operator decision 3
// (HERALD_DEFAULT_RESPONSE_CODEC opt-out).
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	toon "digital.vasic.toon/pkg/toon"
)

// Media-type constants. Kept as exported package-level values so callers
// (Gin handlers, tests) can reference them without re-typing the strings.
const (
	// MediaTypeTOON is the canonical Content-Type for TOON-encoded bodies.
	// Mirrors digital.vasic.toon/pkg/toon.ContentType (the smoke test in
	// toon_smoke_test.go pins value equality on the upstream constant).
	MediaTypeTOON = "application/toon"

	// MediaTypeJSON is the canonical Content-Type for JSON-encoded bodies.
	MediaTypeJSON = "application/json"
)

// EnvDefaultResponseCodec is the operator-facing environment-variable name
// controlling the *default* response codec when the client's Accept header
// is `*/*` or absent. Values: "toon" (default), "json". Anything else
// silently falls back to "toon" — the Herald default per W4b operator
// decision 1.
const EnvDefaultResponseCodec = "HERALD_DEFAULT_RESPONSE_CODEC"

// NegotiateContentType inspects an inbound HTTP request's Accept +
// Content-Type headers and decides:
//
//   - responseCT: the Content-Type the server SHOULD set on the response
//     body — determined by the Accept header per the policy ladder above.
//   - requestCT:  the Content-Type the server SHOULD use to decode the
//     request body — determined by the Content-Type header (defaulting to
//     JSON when absent or `*/*`).
//
// The function is pure (no Gin/net.http coupling) and reads ONLY the env
// var HERALD_DEFAULT_RESPONSE_CODEC for its `*/*` / empty-Accept fallback
// decision (operator opt-out per W4b decision 3).
//
// Header parsing follows RFC 7231 §5.3.2 (media-range + optional `q`
// parameter). Wildcards beyond `*/*` (e.g., `application/*`) are treated
// as `*/*` because Herald only ever serves application/{toon,json}.
//
// The function NEVER returns an empty responseCT/requestCT — a caller is
// always given a concrete content-type it can encode/decode against. The
// safety-fallback when the client asks for something we can't speak
// (e.g., `Accept: application/xml`) is JSON, NOT a lie: we return
// "application/json" so the eventual response header matches the bytes
// we actually emit. The original 2026-05-17 PASS-bluff revision served
// JSON bytes under `Content-Type: application/toon`; that is structurally
// impossible at this layer because the codec dispatch in MarshalChosen
// keys on the SAME string returned here.
func NegotiateContentType(acceptHeader, contentTypeHeader string) (responseCT, requestCT string) {
	responseCT = negotiateResponseFromAccept(acceptHeader)
	requestCT = negotiateRequestFromContentType(contentTypeHeader)
	return responseCT, requestCT
}

// negotiateResponseFromAccept implements the Accept-header policy ladder.
// Separated from NegotiateContentType so the two header inputs can be
// reasoned about independently.
func negotiateResponseFromAccept(acceptHeader string) string {
	trimmed := strings.TrimSpace(acceptHeader)
	if trimmed == "" {
		return defaultResponseCT()
	}

	bestTOON := -1.0
	bestJSON := -1.0
	sawWildcard := false
	sawSupported := false

	for _, part := range strings.Split(trimmed, ",") {
		mt, q := parseAcceptPart(part)
		switch mt {
		case MediaTypeTOON:
			sawSupported = true
			if q > bestTOON {
				bestTOON = q
			}
		case MediaTypeJSON:
			sawSupported = true
			if q > bestJSON {
				bestJSON = q
			}
		case "*/*", "application/*":
			sawWildcard = true
		}
	}

	switch {
	case bestTOON > bestJSON:
		return MediaTypeTOON
	case bestJSON > bestTOON:
		return MediaTypeJSON
	case bestTOON >= 0 && bestJSON >= 0:
		// Tie between equal explicit q-values — Herald-default tie-break
		// per W4b operator decision 1: TOON wins.
		return MediaTypeTOON
	case bestTOON >= 0:
		return MediaTypeTOON
	case bestJSON >= 0:
		return MediaTypeJSON
	case sawWildcard:
		return defaultResponseCT()
	case !sawSupported:
		// Client asked only for things we can't speak. Honest fallback:
		// we ARE going to send JSON bytes — advertise so.
		return MediaTypeJSON
	}
	return defaultResponseCT()
}

// negotiateRequestFromContentType maps an inbound Content-Type header to
// the codec the server should use to decode the body. Defaults to JSON
// when the header is absent, empty, or `*/*` (curl -d '...' usage).
func negotiateRequestFromContentType(contentType string) string {
	mt := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	switch strings.ToLower(mt) {
	case MediaTypeTOON:
		return MediaTypeTOON
	case MediaTypeJSON, "", "*/*":
		return MediaTypeJSON
	}
	// Unknown Content-Type — default to JSON for backwards-compat. The
	// downstream UnmarshalChosen surfaces an explicit error if the bytes
	// don't actually parse as JSON, so the caller still learns about the
	// mismatch instead of silently corrupting state.
	return MediaTypeJSON
}

// parseAcceptPart returns the (media-type, q) pair for a single Accept
// header entry such as "application/toon;q=0.5". q defaults to 1.0 when
// absent or malformed (RFC 7231 §5.3.1).
func parseAcceptPart(part string) (string, float64) {
	segs := strings.Split(part, ";")
	mediaType := strings.ToLower(strings.TrimSpace(segs[0]))
	q := 1.0
	for _, s := range segs[1:] {
		kv := strings.SplitN(strings.TrimSpace(s), "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(kv[0])) != "q" {
			continue
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64); err == nil {
			q = v
		}
	}
	return mediaType, q
}

// defaultResponseCT resolves the Herald-default response Content-Type for
// the `*/*` / empty-Accept rung. Reads HERALD_DEFAULT_RESPONSE_CODEC at
// every call (cheap; no observed perf concern) so tests can flip the env
// per case without needing a process restart. Per W4b operator decision
// 3, "toon" is the default; "json" is the operator opt-out; any other
// value silently falls back to "toon".
func defaultResponseCT() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvDefaultResponseCodec)))
	switch v {
	case "json":
		return MediaTypeJSON
	case "toon", "":
		return MediaTypeTOON
	}
	return MediaTypeTOON
}

// MarshalChosen serialises v using the codec identified by contentType.
// Returns the encoded bytes or an error.
//
// Supported content-types:
//   - "application/toon" → digital.vasic.toon/pkg/toon.Marshal (the REAL
//     upstream TOON wire-up via github.com/toon-format/toon-go; never JSON
//     bytes wearing a TOON content-type — §107 anti-bluff).
//   - "application/json" → encoding/json.Marshal.
//
// Any other content-type returns an explicit error naming the unsupported
// CT. The error message includes the bad value so observability captures
// it without the caller having to log separately.
//
// §107 anti-bluff: callers MUST verify that the returned bytes match the
// claimed content-type — the paired test asserts the TOON bytes do not
// begin with '{' or '[' (the original 2026-05-17 PASS-bluff signature).
func MarshalChosen(v any, contentType string) ([]byte, error) {
	mt := canonicalCT(contentType)
	switch mt {
	case MediaTypeTOON:
		b, err := toon.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("contentnego: toon marshal: %w", err)
		}
		return b, nil
	case MediaTypeJSON:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("contentnego: json marshal: %w", err)
		}
		return b, nil
	}
	return nil, fmt.Errorf("contentnego: unsupported content-type %q for marshal (supported: %s, %s)",
		contentType, MediaTypeTOON, MediaTypeJSON)
}

// UnmarshalChosen decodes data into v using the codec identified by
// contentType. v MUST be a non-nil pointer.
//
// Supported content-types: same as MarshalChosen — application/toon
// (digital.vasic.toon/pkg/toon.Unmarshal) and application/json
// (encoding/json.Unmarshal).
//
// Any other content-type returns an explicit error naming the
// unsupported CT (per the §107 anti-bluff rule: never silently fall back
// to JSON when the caller named a CT we can't handle — that would mask a
// misrouted body).
func UnmarshalChosen(data []byte, v any, contentType string) error {
	if v == nil {
		return errors.New("contentnego: nil target value for unmarshal")
	}
	mt := canonicalCT(contentType)
	switch mt {
	case MediaTypeTOON:
		if err := toon.Unmarshal(data, v); err != nil {
			return fmt.Errorf("contentnego: toon unmarshal: %w", err)
		}
		return nil
	case MediaTypeJSON:
		if err := json.Unmarshal(data, v); err != nil {
			return fmt.Errorf("contentnego: json unmarshal: %w", err)
		}
		return nil
	}
	return fmt.Errorf("contentnego: unsupported content-type %q for unmarshal (supported: %s, %s)",
		contentType, MediaTypeTOON, MediaTypeJSON)
}

// canonicalCT strips RFC 7231 media-type parameters (e.g.,
// "; charset=utf-8") and lowercases the base type. Returns the bare media
// type so the switch in MarshalChosen / UnmarshalChosen can match it
// against MediaTypeTOON / MediaTypeJSON without surprises from parameter
// drift between callers.
func canonicalCT(contentType string) string {
	base := strings.SplitN(contentType, ";", 2)[0]
	return strings.ToLower(strings.TrimSpace(base))
}
