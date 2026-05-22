// Package herald is qaherald's HTTPS client for Herald flavor binaries'
// `/v1/*` business routes (pherald /v1/events, cherald /v1/compliance,
// sherald /v1/safety_state).
//
// Wave 5 Task 4 (2026-05-22). Composes with:
//   - Wave 4a HTTP/3+brotli substrate (TLS 1.3 minimum; ALPN preference
//     "h3,h2"; Accept-Encoding: br on every outbound request).
//   - Wave 4b TOON content negotiation (Accept: application/toon ↔
//     Content-Type: application/toon; real digital.vasic.toon codec
//     via commons/cli/contentnego.go's MediaTypeTOON constant).
//   - commons_auth HMAC mode (HS256 with pre-shared secret; the
//     dev-mode JWT scheme pherald + cherald + sherald already verify
//     against — see commons_auth/hmac.go + claims.go).
//
// §107 anti-bluff anchor (load-bearing):
//
//  1. PostEvent / GetCompliance / GetSafety return the actual HTTP
//     status code + response headers VERBATIM. There is no clamping to
//     202 / 200. T10 mutation gate (b) plants a `return 202` shortcut;
//     the deny-path scenario (T5 scenario_4) expects 403 and the
//     mutation surfaces because the assertion `status == 403` fails.
//
//  2. decode() inspects the response's Content-Type header to pick
//     TOON vs JSON. A mutation that hard-codes the JSON path would
//     surface when the test server returns TOON bytes under an
//     application/toon Content-Type and the test's
//     `decoded.EventID == server.EventID` assertion fails (TOON bytes
//     never start with `{` and `json.Unmarshal` would error).
//
//  3. The JWT issuer/subject claims are visible at construction time;
//     the test asserts `Authorization` header is non-empty so a
//     mutation that nilled the signing path would fail there.
//
// JWT scheme (mirrors commons_auth defaults):
//
//   - Signing method: HS256.
//   - Required claims (per commons_auth/verifier.go defaultRequiredClaims):
//     "tenant" + "sub". Both populated by jwt() below as non-empty
//     strings; commons_auth's hmacVerifier.Verify rejects missing /
//     empty claims with `token missing claim` / `token claim … is empty`.
//   - Standard registered claims: `iss=qaherald`, `exp` 5 minutes
//     ahead of now. Issuer is informational (commons_auth does not
//     audience-restrict in HMAC mode).
//
// The package deliberately exposes a SINGLE construction path —
// New(baseURL, jwtSecret) — because scenarios share the same client
// across invocations. The http.Client is preconfigured with the TLS
// + ALPN posture; tests override `c.http` via `c.http = srv.Client()`
// to share httptest's self-signed TLS trust.
package herald

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	toon "digital.vasic.toon/pkg/toon"
	"github.com/golang-jwt/jwt/v5"
)

// Client is the qaherald-side HTTPS client for Herald /v1/* routes.
// Construct via New; never zero-initialise — the http.Client field is
// required and the TLS config inside it is the substrate posture for
// Wave 4a (TLS 1.3 + h2 preferred; h3 is attempted out-of-band by the
// commons/cli/h3.go listener on the server side, not here).
type Client struct {
	baseURL string
	secret  []byte
	http    *http.Client
}

// CloudEvent is the qaherald-side mirror of the CloudEvents v1.0
// envelope pherald accepts on POST /v1/events. Field tags align with
// pherald/internal/http/events.go so a round-trip through encoding/json
// produces a wire-identical body. The `data` vs `data_base64` split is
// preserved per CloudEvents §3.1 (one of, never both).
type CloudEvent struct {
	SpecVersion string          `json:"specversion"`
	ID          string          `json:"id"`
	Source      string          `json:"source"`
	Type        string          `json:"type"`
	Time        time.Time       `json:"time"`
	DataB64     string          `json:"data_base64,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// Receipt is the qaherald-side mirror of pherald's POST /v1/events
// response body. EventID is the canonical correlation handle scenarios
// use to chase the downstream Telegram delivery.
type Receipt struct {
	EventID    string `json:"event_id"`
	Recipients int    `json:"recipients"`
	Status     string `json:"status"`
	Note       string `json:"note,omitempty"`
}

// Content-Type constants. Kept distinct from commons/cli.MediaType*
// because qaherald should not pull in the commons/cli package's Gin
// dependency just for two string constants. Wire equality is checked
// at test time (the toon-smoke test in commons/cli pins the upstream
// constant); diverging here would surface as a content-negotiation
// mismatch under the round-trip test.
const (
	AcceptTOON = "application/toon"
	AcceptJSON = "application/json"
)

// New constructs a Client bound to baseURL with the supplied HMAC
// secret for JWT signing. The default http.Client carries Wave 4a TLS
// posture (TLS 1.3 minimum; ALPN h2,http/1.1 — h3 is out-of-band).
//
// Tests override `c.http` to swap in httptest.NewTLSServer's
// preconfigured client (which shares the test server's self-signed
// trust); production callers never override.
func New(baseURL string, jwtSecret []byte) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  jwtSecret,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS13,
					NextProtos: []string{"h2", "http/1.1"}, // h3 attempted separately by commons/cli/h3.go on the server side
				},
			},
		},
	}
}

// jwt mints a HS256 token with the commons_auth-required claims
// ("tenant" + "sub") + a 5-minute expiry. Issuer is set to "qaherald"
// for observability — commons_auth does not gate on it.
//
// Tenant is a Herald-canonical zero UUID (the dev-mode default
// pherald + cherald + sherald accept in operator-onboarding flows); a
// real run-time injection MUST replace this with the operator's
// tenant UUID at scenario-orchestration time. The fixed value here
// keeps unit tests deterministic; T7 (run subcommand wiring) will
// thread the operator-supplied tenant through.
func (c *Client) jwt() (string, error) {
	claims := jwt.MapClaims{
		"iss":    "qaherald",
		"sub":    "qa-bot",
		"tenant": "00000000-0000-0000-0000-000000000000",
		"exp":    time.Now().Add(5 * time.Minute).Unix(),
		"iat":    time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(c.secret)
}

// PostEvent dispatches a CloudEvent to <base>/v1/events with the
// requested codec (Accept + Content-Type both = accept). Returns the
// decoded Receipt, raw HTTP status, response headers, and any error.
//
// Surfacing the actual status code (no clamping) is the §107 hook:
// the deny-path scenario (T5 #4) expects 403 and would fail if a
// mutation pinned the return to 202.
func (c *Client) PostEvent(ctx context.Context, ce CloudEvent, accept string) (Receipt, int, http.Header, error) {
	tok, err := c.jwt()
	if err != nil {
		return Receipt{}, 0, nil, fmt.Errorf("jwt: %w", err)
	}
	body, contentType, err := encode(ce, accept)
	if err != nil {
		return Receipt{}, 0, nil, fmt.Errorf("encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/events", bytes.NewReader(body))
	if err != nil {
		return Receipt{}, 0, nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Encoding", "br")
	resp, err := c.http.Do(req)
	if err != nil {
		return Receipt{}, 0, nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Receipt{}, resp.StatusCode, resp.Header, fmt.Errorf("read body: %w", err)
	}
	var r Receipt
	if len(raw) > 0 {
		if err := decode(raw, resp.Header.Get("Content-Type"), &r); err != nil {
			return Receipt{}, resp.StatusCode, resp.Header, fmt.Errorf("decode receipt: %w", err)
		}
	}
	return r, resp.StatusCode, resp.Header, nil
}

// GetCompliance issues a GET on <base>/v1/compliance with the
// requested response codec. Returns the body as a json.RawMessage
// regardless of wire codec — TOON responses are transcoded through
// the real digital.vasic.toon codec into an `interface{}` and then
// json.Marshal-ed so downstream scenario assertions can use standard
// JSONPath/jq idioms against a stable shape. The raw status code +
// headers are surfaced verbatim (§107 anti-bluff hook).
func (c *Client) GetCompliance(ctx context.Context, accept string) (json.RawMessage, int, http.Header, error) {
	return c.getJSON(ctx, "/v1/compliance", accept)
}

// GetSafety issues a GET on <base>/v1/safety_state. See GetCompliance
// for the codec-handling contract.
func (c *Client) GetSafety(ctx context.Context, accept string) (json.RawMessage, int, http.Header, error) {
	return c.getJSON(ctx, "/v1/safety_state", accept)
}

// getJSON is the shared GET helper for the two read-only routes.
// Extracting it keeps the deny-path / status-pass-through invariant
// in one place (a future mutation that clamps status would surface in
// both GetCompliance and GetSafety simultaneously, not just one).
func (c *Client) getJSON(ctx context.Context, path, accept string) (json.RawMessage, int, http.Header, error) {
	tok, err := c.jwt()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("jwt: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Encoding", "br")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("read body: %w", err)
	}
	out, err := normaliseToJSON(raw, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("normalise body: %w", err)
	}
	return out, resp.StatusCode, resp.Header, nil
}

// encode renders v into the requested wire codec. For TOON the real
// digital.vasic.toon codec is invoked (per Wave 4b §107 anti-bluff —
// no JSON-bytes-under-application/toon path); for JSON the standard
// library does. Returns the bytes + the canonical Content-Type to
// stamp on the outbound request.
func encode(v any, contentType string) ([]byte, string, error) {
	switch contentType {
	case AcceptTOON:
		b, err := toon.Marshal(v)
		if err != nil {
			return nil, "", fmt.Errorf("toon.Marshal: %w", err)
		}
		return b, AcceptTOON, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, "", fmt.Errorf("json.Marshal: %w", err)
		}
		return b, AcceptJSON, nil
	}
}

// decode unmarshals raw into dst. The codec is selected from the
// response's Content-Type header — toon.IsTOONContentType is the
// canonical check (case-insensitive substring match per the upstream
// helper). A mutation that hard-coded the JSON path would surface
// under the round-trip test: TOON bytes do not begin with `{` and
// json.Unmarshal would error.
func decode(raw []byte, contentType string, dst any) error {
	if toon.IsTOONContentType(contentType) {
		return toon.Unmarshal(raw, dst)
	}
	return json.Unmarshal(raw, dst)
}

// normaliseToJSON returns a json.RawMessage representation of raw
// regardless of the wire codec. The TOON branch decodes into a
// codec-neutral interface{} and re-encodes via json.Marshal so the
// scenario layer can treat every body as JSON. JSON bodies pass
// through unchanged (after a defensive json.RawMessage copy).
func normaliseToJSON(raw []byte, contentType string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if toon.IsTOONContentType(contentType) {
		var generic any
		if err := toon.Unmarshal(raw, &generic); err != nil {
			return nil, fmt.Errorf("toon.Unmarshal: %w", err)
		}
		out, err := json.Marshal(generic)
		if err != nil {
			return nil, fmt.Errorf("json.Marshal after toon decode: %w", err)
		}
		return json.RawMessage(out), nil
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return json.RawMessage(cp), nil
}
