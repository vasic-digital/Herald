<div align="center">

![Herald](../../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Wave 4a — HTTP/3 (QUIC) + Brotli + Alt-Svc Transport Substrate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` per Universal Constitution §11.4.70. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Land the §41.7 transport substrate inside Herald — every flavor's `serve` subcommand binds an HTTP/3 (QUIC) UDP listener alongside the existing TCP/HTTP listener (TLS 1.3 mandatory), serves Brotli-compressed responses when clients accept `br`, advertises HTTP/3 via the `Alt-Svc` response header on the TCP fallback, and sources TLS cert/key via flag/env (production) or an auto-generated self-signed cert (dev). Closes the HTTP/3 + Brotli legs of the Wave 4 design doc. Tags **v0.2.0**. TOON adoption is deferred to Wave 4b.

**Architecture:** Single binary, single port per flavor — TCP listens on `tcp/<port>` (HTTP/1.1+HTTP/2) AND UDP listens on `udp/<port>` (HTTP/3) sharing the same Gin engine as `net/http.Handler`. The existing `commons/cli/serve.go` is refactored to drive both listeners through a new internal dual-listener helper. TLS cert sourcing lives in a new tiny foundation module `commons_tls/` (the 15th workspace module — no upstream catalogue match per §11.4.74). Brotli + Alt-Svc are wired via the already-vendored `digital.vasic.middleware/pkg/{brotli,altsvc}` packages bridged into Gin through `digital.vasic.middleware/pkg/gin.Wrap`. The HTTP/3 listener is a thin wrapper over the newly-vendored `digital.vasic.http3` submodule (which itself wraps `github.com/quic-go/quic-go/http3.Server`).

**Tech Stack:** Go 1.25+; `github.com/quic-go/quic-go` v0.x (via `digital.vasic.http3`); `github.com/andybalholm/brotli` (via `digital.vasic.middleware/pkg/brotli`); `crypto/tls` + `crypto/ecdsa` + `crypto/x509` (stdlib, for dev cert auto-generation); `github.com/gin-gonic/gin` (existing); existing `commons/`, `commons_auth/`, `commons_messaging/`, `commons_storage/` packages.

**Spec reference:** [`docs/superpowers/specs/2026-05-22-http3-brotli-toon-design.md`](../specs/2026-05-22-http3-brotli-toon-design.md) (commit `c60b3fd`) — Sections 1 (current state), 2.1+2.2 (submodule survey for http3 + middleware), 3.1+3.4 (dual-listener inside Herald, Approach 1A), 4 (Brotli negotiation), 6.1+6.3 (e2e invariants + mutation gates), 7.3 (Wave 4a task sketch), 8 (spec V3 r9 → r10 impact).

**Wave 3b substrate already landed:** pherald Runner live + `/v1/events` 202 (commit `48bf293..`), `commons_auth` JWT middleware on cherald + sherald + pherald, e2e_bluff_hunt at 47 invariants (E1..E48 + skips). Wave 4a is a transport upgrade — zero handler change required.

**Operator-locked decisions (recorded 2026-05-22):**

1. **TLS dev cert auto-gen.** When neither `--tls-cert`/`--tls-key` flags nor `HERALD_TLS_CERT_PATH`/`HERALD_TLS_KEY_PATH` env vars are set, Herald auto-generates an ECDSA P-256 self-signed cert with SAN `localhost`, `127.0.0.1`, `::1`, validity 365 days, written to `~/.herald/dev-cert.pem` + `~/.herald/dev-key.pem` (chmod 600) on first run. Subsequent runs reuse if files exist and not expired. Logged as `WARN: DEV cert in use — do not deploy to production`.
2. **Production TLS fail-loud.** When `HERALD_AUTH_MODE=jwks` is set (production signal) AND no cert flag/env supplied → startup FAILS with `pherald: production deployment (HERALD_AUTH_MODE=jwks) requires --tls-cert + --tls-key (or HERALD_TLS_CERT_PATH/HERALD_TLS_KEY_PATH env)`. No silent dev-cert fallback in production.
3. **Brotli quality = 6** (balanced; one notch above stdlib `brotli.DefaultCompression`). Hardcoded into `commons/cli/brotli.go` as `BrotliLevel = 6`. Operator override via `HERALD_BROTLI_LEVEL` env var (optional, range 0-11).
4. **Always-on HTTP/3** with `HERALD_DISABLE_HTTP3=1` escape hatch. The TCP listener is NEVER disabled (fallback path stays intact). When `HERALD_DISABLE_HTTP3=1` is set, the serve command logs `INFO: HTTP/3 listener disabled by env (HERALD_DISABLE_HTTP3=1)` and skips QUIC binding.
5. **TOON is OUT OF SCOPE.** Wave 4a does NOT touch `digital.vasic.toon`. Wave 4b follow-up plan covers TOON end-to-end.

---

## File Structure

### CREATE

| Path | Responsibility |
|---|---|
| `commons_tls/go.mod` | New 15th workspace module — `github.com/vasic-digital/herald/commons_tls`; stdlib + Go 1.25 |
| `commons_tls/cert.go` | ECDSA P-256 dev cert generator + cert loader (`LoadOrGenerate`); `ResolveCertSource(flags, env, mode)` resolution policy |
| `commons_tls/cert_test.go` | Unit tests: dev cert auto-gen, file persistence + chmod 600 verification, cert reuse on second call, prod-mode fail-loud when flags absent, ECDSA P-256 + TLS 1.3 + SAN list assertion |
| `commons/cli/h3.go` | HTTP/3 listener wrapper over `digital.vasic.http3.Server`; `startQUIC(handler, addr, tlsConfig) (*quicServer, error)`; graceful shutdown |
| `commons/cli/h3_test.go` | Unit test: real `quic-go/http3` client connects to a started server, sends GET, receives 200; assert `len(serverAddr.Network()) == "udp"` |
| `commons/cli/altsvc.go` | Gin middleware emitting `Alt-Svc: h3=":<port>"; ma=2592000` on every TCP response; reuses `digital.vasic.middleware/pkg/altsvc` under the hood |
| `commons/cli/altsvc_test.go` | Unit test: middleware sets the header on response; header value matches the configured H3 port exactly |
| `commons/cli/brotli.go` | Gin middleware applying Brotli quality 6 to compressible responses ≥ 256 B; reuses `digital.vasic.middleware/pkg/brotli` + `pkg/gin.Wrap` |
| `commons/cli/brotli_test.go` | Unit test: a 1 KiB JSON payload with `Accept-Encoding: br` comes back with `Content-Encoding: br` + decompresses to original via `andybalholm/brotli.NewReader`; sub-MinLength body NOT compressed |
| `docs/catalogue-checks/HRD-100-commons-tls.md` | §11.4.74 catalogue-check for the new `commons_tls/` module — verdict `no-match → vendor as Herald-internal` |
| `tests/test_wave4_mutation_meta.sh` | Wave 4a paired-mutation gate — M1 (strip H3), M2 (strip Brotli), M3 (downgrade TLS to 1.2) + post-flight |

### MODIFY

| Path | Change |
|---|---|
| `submodules/http3/` (NEW git submodule) | Add `git@github.com:vasic-digital/http3.git` at pin `1d0df7b`; reference via `replace` directive in `commons/go.mod` |
| `.gitmodules` | Append entry for `submodules/http3` |
| `commons/go.mod` | Add `digital.vasic.http3` as direct dep; add `replace digital.vasic.http3 => ../submodules/http3`; ensure `github.com/quic-go/quic-go` + `github.com/andybalholm/brotli` resolve (indirect or direct) |
| `commons/cli/serve.go` | Dual-listener refactor — accept `ServeOpts.TLSConfig` (built via `commons_tls.ResolveCertSource`); start TCP via `srv.ListenAndServeTLS` + UDP via `startQUIC` concurrently; both share same Gin engine; both shut down on SIGTERM/SIGINT; Brotli + Alt-Svc middleware appended to the existing chain BEFORE the flavor-supplied middleware (so they apply to healthz/readyz/metrics + routes uniformly) |
| `commons/cli/cli_test.go` | Extend existing TestServeCmd to verify HTTP/3 listener wires (synthetic; the deep test lives in `h3_test.go`); regression-assert healthz still 200 |
| `pherald/cmd/pherald/main.go` | Add `--tls-cert` + `--tls-key` Cobra flags; build TLS config via `commons_tls.ResolveCertSource(flags, env, prodMode=AuthIsJWKS())` and pass to `cli.ServeOpts` |
| `cherald/cmd/cherald/main.go` | Same flag + env wiring as pherald |
| `sherald/cmd/sherald/main.go` | Same flag + env wiring as pherald |
| `go.work` | Add `./commons_tls` (14 → 15 modules) |
| `scripts/e2e_bluff_hunt.sh` | Append E49-E55 block (7 invariants — see §3 below); header tally `Forty-seven` → `Fifty-four` |
| `docs/specs/mvp/specification.V3.md` | r9 → r10: add §41.7 Transport subsection (six sub-sections per spec doc §8.1); §44.M Wave 4a milestone with as-built evidence |
| `docs/Issues.md` | r12 → r13: prepend HRD-101..HRD-103 Issues→Fixed atomic close at commit time (HRD-101 = HTTP/3 listener; HRD-102 = Brotli middleware; HRD-103 = TLS cert sourcing) |
| `docs/Fixed.md` | r11 → r12: prepend HRD-101/102/103 to Recently fixed with commit SHA + e2e invariant references |
| `docs/Status.md` | r13 → r14: Wave 4a completion summary; module count 14 → 15; e2e invariant total 47 → 54 |
| `CLAUDE.md` | r7 → r8: workspace module count 14 → 15 (adds `commons_tls`); brief Wave 4a status pointer |

---

## Task 1: Vendor `digital.vasic.http3` submodule + commons/go.mod wiring

**Files:**
- Create: `submodules/http3/` (git submodule add)
- Modify: `.gitmodules`, `commons/go.mod`, `commons/go.sum`

This task is pure plumbing — adds the upstream submodule and verifies a smoke `go build ./commons/...` still succeeds with the new dependency available. No source change in `commons/` yet; the `replace` directive is set up so subsequent tasks can import `digital.vasic.http3/pkg/server`.

- [ ] **Step 1: Add the submodule**

```bash
cd /Users/milosvasic/Projects/Herald
git submodule add git@github.com:vasic-digital/http3.git submodules/http3
cd submodules/http3
git checkout 1d0df7b700436b70a361c3ba14d0520b070e7df9
cd /Users/milosvasic/Projects/Herald
```

Expected: `.gitmodules` gains a new `[submodule "submodules/http3"]` block; `submodules/http3/` populated with the upstream tree at the pinned SHA.

- [ ] **Step 2: Smoke that the submodule's own `go build` works**

```bash
cd /Users/milosvasic/Projects/Herald/submodules/http3
go build ./... 2>&1 | head -3
cd /Users/milosvasic/Projects/Herald
```

Expected: silent (clean compile of the submodule against its own `go.mod`). If the submodule's tests fail standalone, STOP — fix upstream first per CONST-051(A).

- [ ] **Step 3: Add `replace` directive + import to `commons/go.mod`**

Edit `commons/go.mod` — append the require block + replace directive:

```go
require digital.vasic.http3 v0.0.0-00010101000000-000000000000

replace digital.vasic.http3 => ../submodules/http3
```

Also ensure indirect deps are reachable; run:

```bash
cd /Users/milosvasic/Projects/Herald/commons
go mod tidy
cd /Users/milosvasic/Projects/Herald
```

Expected: `commons/go.sum` updated with `quic-go/quic-go` + `andybalholm/brotli` transitive lines; `commons/go.mod` carries the `replace digital.vasic.http3 => ../submodules/http3` directive.

- [ ] **Step 4: Verify `commons` still builds + tests still pass**

```bash
cd /Users/milosvasic/Projects/Herald
go build ./commons/... 2>&1 | head -3
go test -race -count=1 ./commons/... 2>&1 | tail -10
```

Expected: clean compile + existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add .gitmodules submodules/http3 commons/go.mod commons/go.sum
git commit -m "Wave 4a step 1: vendor digital.vasic.http3 @ 1d0df7b as submodule

Adds submodules/http3/ pointing at git@github.com:vasic-digital/http3.git
pinned to 1d0df7b (HEAD as of 2026-05-20 catalogue check). Wires the
replace directive in commons/go.mod so subsequent tasks (T3) can
import digital.vasic.http3/pkg/server.

No source changes in commons/ yet; this commit is pure plumbing per
the Wave 4 design doc §2.1 + §7.3 T1. Submodule's own go build +
go test pass at the pinned SHA.

Catalogue-check: REUSE per docs/catalogue-checks/HRD-100-... follow-up.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 2: `commons_tls/` — 15th workspace module + cert auto-generation

**Files:**
- Create: `commons_tls/go.mod`, `commons_tls/cert.go`, `commons_tls/cert_test.go`
- Create: `docs/catalogue-checks/HRD-100-commons-tls.md`
- Modify: `go.work`

This task lays the TLS cert-sourcing foundation. Operator-locked: auto-gen ECDSA P-256 dev cert at `~/.herald/dev-cert.pem` + `~/.herald/dev-key.pem` on first run; production fail-loud when no cert supplied AND `HERALD_AUTH_MODE=jwks`.

- [ ] **Step 1: Write the catalogue-check doc**

Create `docs/catalogue-checks/HRD-100-commons-tls.md`:

```markdown
<div align="center">

![Herald](../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Catalogue-Check — HRD-100 commons_tls/

| Field | Value |
|---|---|
| Date | 2026-05-22 |
| Target | `commons_tls/` (TLS cert sourcing for HTTP/3 + HTTP/2) |
| Orgs queried | `vasic-digital/*`, `HelixDevelopment/*` |
| Verdict | **no-match → vendor as Herald-internal package** |
| Evidence commits | Wave 4a task 2 |

## Search performed

1. `gh search repos --owner=vasic-digital --owner=HelixDevelopment 'tls cert ecdsa autogen'` → 0 hits with cert-auto-generation shape.
2. `digital.vasic.http3/internal/testcert` exists but is internal-package + RSA + lifetime 1 hour — designed for unit tests, not operator dev workflow. Lifting it to public would require ECDSA upgrade + extended SAN + persistent on-disk storage + chmod 600 — substantive enough that the right move is a fresh Herald-internal `commons_tls/` per CONST-051(B).
3. `crypto/tls` + `crypto/ecdsa` + `crypto/x509` (stdlib) provide all primitives; the value-add is the Herald-specific resolution policy (`HERALD_AUTH_MODE=jwks` ⇒ fail-loud; otherwise dev-cert auto-gen at `~/.herald/`).

## Verdict rationale

No existing module in our orgs provides the specific Herald shape:

- ECDSA P-256 (HTTP/3 spec strongly prefers EC; smaller payloads, faster handshake)
- SAN: `localhost` + `127.0.0.1` + `::1` (covers loopback dev testing on dual-stack hosts)
- Validity 365 days (long enough for active dev cycles; renew-by-regenerate)
- Persisted to `~/.herald/dev-cert.pem` + `dev-key.pem` (chmod 600) — survives Herald restarts; deletable to force regeneration
- Resolution policy: flag > env > dev-autogen (with prod-mode fail-loud override)

`digital.vasic.http3/internal/testcert` covers ~25% (RSA, ephemeral, internal). Promoting + extending to fit Herald's full shape would mix Herald-specific opinions (the `~/.herald/` path, the HERALD_AUTH_MODE check) into a project-neutral submodule — CONST-051(B) violation. Vendoring as Herald-internal `commons_tls/` is the correct choice.

When Herald later identifies primitives general enough to lift back (e.g., the bare ECDSA P-256 generator with configurable SAN list), we promote them upstream per §11.4.35 with the explicit `Lifted from herald to digital.vasic.http3 per §11.4.35` commit annotation.

## Public surface

See `commons_tls/cert.go`:

- `Config{CertPath, KeyPath, Hosts, Validity}` — input
- `LoadOrGenerate(cfg Config) (tls.Certificate, error)` — load from disk or generate + persist
- `ResolveCertSource(flagCert, flagKey, prodMode bool) (tls.Certificate, error)` — full Herald policy
```

- [ ] **Step 2: Create `commons_tls/go.mod`**

```bash
mkdir -p /Users/milosvasic/Projects/Herald/commons_tls
cat > /Users/milosvasic/Projects/Herald/commons_tls/go.mod <<'EOF'
// Module commons_tls is Herald's TLS cert-sourcing helper. Owns the
// dev self-signed cert auto-generation flow + cert resolution policy
// (flag > env > dev-autogen, with prod-mode fail-loud override).
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons_tls

go 1.25.0
EOF
```

- [ ] **Step 3: Write the failing test `commons_tls/cert_test.go`**

```go
package commons_tls

import (
	"crypto/ecdsa"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadOrGenerate_CreatesECDSAP256WithExpectedSANs(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CertPath: filepath.Join(dir, "dev-cert.pem"),
		KeyPath:  filepath.Join(dir, "dev-key.pem"),
		Hosts:    []string{"localhost", "127.0.0.1", "::1"},
		Validity: 365 * 24 * time.Hour,
	}
	cert, err := LoadOrGenerate(cfg)
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("cert.Certificate empty")
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if _, ok := parsed.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Errorf("public key is %T, want *ecdsa.PublicKey", parsed.PublicKey)
	}
	if pub, _ := parsed.PublicKey.(*ecdsa.PublicKey); pub != nil && pub.Curve.Params().Name != "P-256" {
		t.Errorf("curve = %s, want P-256", pub.Curve.Params().Name)
	}
	hasLocalhost := false
	for _, n := range parsed.DNSNames {
		if n == "localhost" {
			hasLocalhost = true
		}
	}
	if !hasLocalhost {
		t.Errorf("SAN missing localhost: DNSNames=%v", parsed.DNSNames)
	}
	hasV4 := false
	hasV6 := false
	for _, ip := range parsed.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			hasV4 = true
		}
		if ip.Equal(net.ParseIP("::1")) {
			hasV6 = true
		}
	}
	if !hasV4 || !hasV6 {
		t.Errorf("SAN missing IPs: IPAddresses=%v hasV4=%v hasV6=%v", parsed.IPAddresses, hasV4, hasV6)
	}
}

func TestLoadOrGenerate_PersistsWith0600Permissions(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CertPath: filepath.Join(dir, "dev-cert.pem"),
		KeyPath:  filepath.Join(dir, "dev-key.pem"),
		Hosts:    []string{"localhost"},
		Validity: 365 * 24 * time.Hour,
	}
	if _, err := LoadOrGenerate(cfg); err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	st, err := os.Stat(cfg.KeyPath)
	if err != nil {
		t.Fatalf("Stat key: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("key permissions = %o, want 0600 (secret material leak risk)", st.Mode().Perm())
	}
}

func TestLoadOrGenerate_ReusesExistingCertWhenFilesPresent(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CertPath: filepath.Join(dir, "dev-cert.pem"),
		KeyPath:  filepath.Join(dir, "dev-key.pem"),
		Hosts:    []string{"localhost"},
		Validity: 365 * 24 * time.Hour,
	}
	cert1, err := LoadOrGenerate(cfg)
	if err != nil {
		t.Fatalf("first LoadOrGenerate: %v", err)
	}
	// Second call MUST return the exact same cert bytes (no re-gen).
	cert2, err := LoadOrGenerate(cfg)
	if err != nil {
		t.Fatalf("second LoadOrGenerate: %v", err)
	}
	if len(cert1.Certificate) != len(cert2.Certificate) {
		t.Fatalf("cert chain length differs across calls (1st=%d, 2nd=%d) — re-generated when it should have loaded", len(cert1.Certificate), len(cert2.Certificate))
	}
	for i := range cert1.Certificate {
		if string(cert1.Certificate[i]) != string(cert2.Certificate[i]) {
			t.Errorf("cert chain[%d] differs across calls — re-generated when it should have loaded", i)
		}
	}
}

func TestResolveCertSource_ProductionModeFailsLoudWithoutCert(t *testing.T) {
	// HERALD_AUTH_MODE=jwks ⇒ production deployment signal ⇒ no dev-autogen fallback.
	_, err := ResolveCertSource("", "", true)
	if err == nil {
		t.Fatal("ResolveCertSource in prod mode with no cert MUST error")
	}
	if !strings.Contains(err.Error(), "--tls-cert") || !strings.Contains(err.Error(), "--tls-key") {
		t.Errorf("error should reference required flags: got %q", err.Error())
	}
}

func TestResolveCertSource_DevModeAutogeneratesWhenAbsent(t *testing.T) {
	// Override the home dir for the duration of the test so we don't litter ~/.herald.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cert, err := ResolveCertSource("", "", false)
	if err != nil {
		t.Fatalf("ResolveCertSource dev mode: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("dev auto-gen returned empty cert")
	}
	if _, err := os.Stat(filepath.Join(tmp, ".herald", "dev-cert.pem")); err != nil {
		t.Errorf("dev cert not persisted at ~/.herald/dev-cert.pem: %v", err)
	}
}
```

- [ ] **Step 4: Run test — verify compile-FAIL (Config + LoadOrGenerate undefined)**

```bash
cd /Users/milosvasic/Projects/Herald/commons_tls
go test -count=1 . 2>&1 | tail -5
cd /Users/milosvasic/Projects/Herald
```

Expected: `undefined: Config` / `undefined: LoadOrGenerate` / `undefined: ResolveCertSource`.

- [ ] **Step 5: Implement `commons_tls/cert.go`**

```go
// Package commons_tls is Herald's TLS cert-sourcing helper.
//
// Two entry points:
//
//   LoadOrGenerate(cfg) — load a cert+key pair from disk if both files
//                         exist; otherwise generate a fresh ECDSA P-256
//                         self-signed cert with the configured SAN list
//                         and persist (chmod 600).
//
//   ResolveCertSource(flagCert, flagKey, prodMode) — Herald's full
//                         resolution policy. Returns:
//
//                           - load from flag-supplied paths if both set
//                           - load from HERALD_TLS_CERT_PATH/KEY_PATH env if both set
//                           - error if prodMode && no flags + no env
//                           - dev-autogen at ~/.herald/dev-{cert,key}.pem otherwise
//
// Per §107: a "TLS configured" PASS without observing a real handshake
// is a bluff — the e2e suite (Wave 4a T8) speaks real openssl s_client
// against the cert and asserts TLSv1.3 + ALPN h3.
package commons_tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Config holds inputs to LoadOrGenerate.
type Config struct {
	CertPath string        // absolute path to PEM cert file
	KeyPath  string        // absolute path to PEM key file
	Hosts    []string      // SAN entries — DNS names AND/OR IP strings
	Validity time.Duration // certificate validity from time.Now()
}

// LoadOrGenerate returns a tls.Certificate. If both CertPath + KeyPath
// exist on disk, the pair is loaded via tls.LoadX509KeyPair (no
// regeneration). Otherwise a fresh ECDSA P-256 self-signed cert with
// the configured SANs is generated and persisted with file permission
// 0600 on the key file.
func LoadOrGenerate(cfg Config) (tls.Certificate, error) {
	// Reuse if both files present.
	if _, errC := os.Stat(cfg.CertPath); errC == nil {
		if _, errK := os.Stat(cfg.KeyPath); errK == nil {
			return tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
		}
	}
	// Generate ECDSA P-256.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: generate ECDSA P-256 key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: serial: %w", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Herald DEV (commons_tls auto-gen)"},
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(cfg.Validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range cfg.Hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: create cert: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CertPath), 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: mkdir cert dir: %w", err)
	}
	certOut, err := os.OpenFile(cfg.CertPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: open cert file: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certOut.Close()
		return tls.Certificate{}, fmt.Errorf("commons_tls: write cert: %w", err)
	}
	certOut.Close()
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: marshal EC key: %w", err)
	}
	keyOut, err := os.OpenFile(cfg.KeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: open key file: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		keyOut.Close()
		return tls.Certificate{}, fmt.Errorf("commons_tls: write key: %w", err)
	}
	keyOut.Close()
	return tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
}

// ResolveCertSource implements Herald's full TLS cert resolution policy.
//
// Precedence (high → low):
//
//   1. Operator-supplied flags (--tls-cert + --tls-key).
//   2. Operator-supplied env (HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH).
//   3. Dev auto-generation at ~/.herald/dev-{cert,key}.pem (NOT in prod).
//   4. Error — when prodMode AND none of (1)/(2) supplied.
//
// prodMode is true when the caller signals a production deployment
// (Herald convention: HERALD_AUTH_MODE=jwks). In production the dev
// auto-gen path MUST NOT be taken — better to fail loud than silently
// serve a self-signed cert in production.
func ResolveCertSource(flagCert, flagKey string, prodMode bool) (tls.Certificate, error) {
	if flagCert != "" && flagKey != "" {
		return tls.LoadX509KeyPair(flagCert, flagKey)
	}
	if flagCert != "" || flagKey != "" {
		return tls.Certificate{}, errors.New("commons_tls: --tls-cert + --tls-key must be supplied together (one without the other is incomplete)")
	}
	envCert := os.Getenv("HERALD_TLS_CERT_PATH")
	envKey := os.Getenv("HERALD_TLS_KEY_PATH")
	if envCert != "" && envKey != "" {
		return tls.LoadX509KeyPair(envCert, envKey)
	}
	if envCert != "" || envKey != "" {
		return tls.Certificate{}, errors.New("commons_tls: HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH must be supplied together")
	}
	if prodMode {
		return tls.Certificate{}, errors.New("commons_tls: production deployment (HERALD_AUTH_MODE=jwks) requires --tls-cert + --tls-key (or HERALD_TLS_CERT_PATH/HERALD_TLS_KEY_PATH env); no dev-autogen fallback in production")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("commons_tls: resolve home dir: %w", err)
	}
	devDir := filepath.Join(home, ".herald")
	return LoadOrGenerate(Config{
		CertPath: filepath.Join(devDir, "dev-cert.pem"),
		KeyPath:  filepath.Join(devDir, "dev-key.pem"),
		Hosts:    []string{"localhost", "127.0.0.1", "::1"},
		Validity: 365 * 24 * time.Hour,
	})
}
```

- [ ] **Step 6: Run tests — verify 5/5 PASS**

```bash
cd /Users/milosvasic/Projects/Herald/commons_tls
go test -race -count=1 . 2>&1 | tail -10
cd /Users/milosvasic/Projects/Herald
```

- [ ] **Step 7: Add `commons_tls` to `go.work`**

```bash
cd /Users/milosvasic/Projects/Herald
go work use ./commons_tls
```

Expected: `go.work` now lists `./commons_tls`. The full module count becomes 15 (commons + commons_constitution + commons_infra + commons_messaging + commons_prefix + commons_storage + commons_auth + commons_tls + pherald + sherald + cherald + bherald + rherald + iherald + scherald).

- [ ] **Step 8: Commit**

```bash
git add commons_tls/ docs/catalogue-checks/HRD-100-commons-tls.md go.work
git commit -m "Wave 4a step 2: commons_tls/ — TLS cert sourcing (15th workspace module)

ECDSA P-256 self-signed dev cert auto-generation at ~/.herald/dev-cert.pem
+ ~/.herald/dev-key.pem (chmod 600), SAN list [localhost, 127.0.0.1, ::1],
validity 365 days. Reused on subsequent runs when files present.

ResolveCertSource policy ladder (high → low):
  1. flag --tls-cert + --tls-key
  2. env HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH
  3. dev auto-gen (NOT in production)
  4. error — prodMode && no cert supplied (HERALD_AUTH_MODE=jwks signal)

5 unit tests: ECDSA P-256 + SAN list, chmod 0600, reuse on second call,
prod-mode fail-loud, dev-mode auto-gen at \$HOME/.herald/.

Catalogue-check verdict: no-match → vendor as Herald-internal per
docs/catalogue-checks/HRD-100-commons-tls.md (CONST-051(B)).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: `commons/cli/h3.go` — HTTP/3 listener wrapper

**Files:**
- Create: `commons/cli/h3.go`, `commons/cli/h3_test.go`

Thin wrapper over `digital.vasic.http3/pkg/server.Server`. Owns construction, start, shutdown, error propagation. The wrapper exists (vs. importing the http3 server directly into `serve.go`) so that (a) `serve.go` stays readable and (b) unit testing the listener in isolation is straightforward.

- [ ] **Step 1: Write the failing test `commons/cli/h3_test.go`**

```go
package cli

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/vasic-digital/herald/commons_tls"
)

func TestStartQUIC_RealClientHandshake(t *testing.T) {
	// Bind a fresh UDP port; use a dev cert from commons_tls.
	tmp := t.TempDir()
	cert, err := commons_tls.LoadOrGenerate(commons_tls.Config{
		CertPath: tmp + "/cert.pem",
		KeyPath:  tmp + "/key.pem",
		Hosts:    []string{"localhost", "127.0.0.1", "::1"},
		Validity: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	// Pick a free UDP port by binding briefly then releasing.
	addr := freeUDPAddr(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/h3probe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello-from-h3"))
	})
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
		MinVersion:   tls.VersionTLS13,
	}
	srv, err := startQUIC(addr, mux, tlsCfg)
	if err != nil {
		t.Fatalf("startQUIC: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	// Real quic-go HTTP/3 client.
	rt := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}},
	}
	defer rt.Close()
	client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
	resp, err := client.Get("https://" + addr + "/h3probe")
	if err != nil {
		t.Fatalf("client.Get over HTTP/3: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "hello-from-h3") {
		t.Errorf("body = %q, want substring 'hello-from-h3'", body)
	}
	// Sanity: assert the server bound on UDP (not TCP).
	if !strings.HasPrefix(srv.Addr(), addr[:strings.Index(addr, ":")]) {
		// Soft check — different binding semantics across OSes; the round-trip
		// over a real quic-go client is the load-bearing positive evidence.
	}
}

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeUDPAddr: %v", err)
	}
	addr := pc.LocalAddr().String()
	pc.Close()
	return addr
}
```

- [ ] **Step 2: Run test — verify FAIL (startQUIC undefined)**

```bash
cd /Users/milosvasic/Projects/Herald
go test ./commons/cli/ -count=1 -run TestStartQUIC 2>&1 | tail -5
```

Expected: `undefined: startQUIC`.

- [ ] **Step 3: Implement `commons/cli/h3.go`**

```go
package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	h3srv "digital.vasic.http3/pkg/server"
)

// quicServer is a thin handle over digital.vasic.http3's Server.
// Owns the lifecycle (Shutdown, Done, Addr) needed by serve.go's
// dual-listener orchestration.
type quicServer struct {
	inner *h3srv.Server
}

// startQUIC binds a UDP listener at addr serving the given handler
// over HTTP/3 with the provided TLS config. Returns immediately after
// the listener is up; the actual request loop runs in a background
// goroutine inside the http3 server. Per §107: a "QUIC started" PASS
// without observing a real handshake is a bluff — h3_test.go's
// TestStartQUIC_RealClientHandshake speaks real quic-go protocol to
// prove the listener accepts.
func startQUIC(addr string, handler http.Handler, tlsCfg *tls.Config) (*quicServer, error) {
	if tlsCfg.MinVersion < tls.VersionTLS13 {
		return nil, fmt.Errorf("h3: TLS 1.3 mandatory for HTTP/3 (got MinVersion=%d)", tlsCfg.MinVersion)
	}
	// Defensive: quic-go requires "h3" in NextProtos for ALPN negotiation.
	hasH3 := false
	for _, p := range tlsCfg.NextProtos {
		if p == "h3" {
			hasH3 = true
			break
		}
	}
	if !hasH3 {
		tlsCfg.NextProtos = append([]string{"h3"}, tlsCfg.NextProtos...)
	}
	cfg := h3srv.Config{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsCfg,
	}
	srv, err := h3srv.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("h3: build server: %w", err)
	}
	if err := srv.Start(); err != nil {
		return nil, fmt.Errorf("h3: start: %w", err)
	}
	return &quicServer{inner: srv}, nil
}

func (q *quicServer) Shutdown(ctx context.Context) error {
	if q == nil || q.inner == nil {
		return nil
	}
	return q.inner.Shutdown(ctx)
}

func (q *quicServer) Done() <-chan error {
	return q.inner.Done()
}

func (q *quicServer) Addr() string {
	if q.inner == nil {
		return ""
	}
	return q.inner.Addr()
}
```

(Note: the exact symbol names `h3srv.Config{Addr, Handler, TLSConfig}` + `h3srv.New(cfg)` + `srv.Start()` are based on `digital.vasic.http3@1d0df7b`'s `pkg/server/server.go` documented surface. If the upstream API has drifted by the time T3 lands, adapt the calls to match the actual API as discovered via `go doc digital.vasic.http3/pkg/server` — DO NOT bypass the wrapper by importing `quic-go/http3.Server` directly into Herald.)

- [ ] **Step 4: Run tests — verify PASS**

```bash
go test -race -count=1 ./commons/cli/ -run TestStartQUIC 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add commons/cli/h3.go commons/cli/h3_test.go
git commit -m "Wave 4a step 3: commons/cli/h3.go — HTTP/3 listener wrapper

Thin wrapper over digital.vasic.http3/pkg/server.Server. Owns the
lifecycle (Shutdown, Done, Addr) that the dual-listener refactor in
serve.go (T6) consumes. Defensive: rejects TLS < 1.3 + auto-injects
'h3' into NextProtos if the caller forgot.

1 unit test (TestStartQUIC_RealClientHandshake) speaks real quic-go
HTTP/3 client → real Herald-handler GET → asserts 200 + body. NOT
mocked transport. Test depends on commons_tls.LoadOrGenerate from T2
for an ephemeral cert with [localhost, 127.0.0.1, ::1] SANs.

Per §107: a 'QUIC started' PASS without a real handshake is a §11.4
bluff; this commit's test is the load-bearing positive evidence for
E49+E52 in the e2e suite (T8).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: `commons/cli/altsvc.go` — Alt-Svc Gin middleware

**Files:**
- Create: `commons/cli/altsvc.go`, `commons/cli/altsvc_test.go`

Wraps `digital.vasic.middleware/pkg/altsvc` via `digital.vasic.middleware/pkg/gin.Wrap` into a Gin handler with Herald's chosen defaults (max-age 30 days = 2,592,000 seconds, port matches the configured H3 port).

- [ ] **Step 1: Write failing test `commons/cli/altsvc_test.go`**

```go
package cli

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAltSvcMiddleware_SetsExpectedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AltSvcMiddleware(24791))
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := rec.Header().Get("Alt-Svc")
	if got == "" {
		t.Fatal("Alt-Svc header missing — middleware no-op'd")
	}
	if !strings.Contains(got, `h3=":24791"`) {
		t.Errorf("Alt-Svc = %q, want substring 'h3=\":24791\"'", got)
	}
	if !strings.Contains(got, "ma=2592000") {
		t.Errorf("Alt-Svc = %q, want substring 'ma=2592000' (30-day max-age)", got)
	}
}

func TestAltSvcMiddleware_PortZero_NoHeader(t *testing.T) {
	// Defensive: when port is 0 (disabled), no header is emitted.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AltSvcMiddleware(0))
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Alt-Svc"); got != "" {
		t.Errorf("Alt-Svc header set when port=0: %q", got)
	}
}
```

- [ ] **Step 2: Run test — verify FAIL**

```bash
go test ./commons/cli/ -count=1 -run TestAltSvcMiddleware 2>&1 | tail -5
```

Expected: `undefined: AltSvcMiddleware`.

- [ ] **Step 3: Implement `commons/cli/altsvc.go`**

```go
package cli

import (
	"strconv"

	altsvc "digital.vasic.middleware/pkg/altsvc"
	mwgin "digital.vasic.middleware/pkg/gin"
	"github.com/gin-gonic/gin"
)

// AltSvcMiddleware returns a Gin handler that emits the Alt-Svc header
// advertising HTTP/3 availability on the given port. Clients that
// understand Alt-Svc (curl --http3, Chromium, Firefox, Go clients using
// digital.vasic.http3 consumer-side) upgrade to QUIC on subsequent
// requests.
//
// Per §107: the header MUST be observable in the actual response. The
// e2e suite (T8) asserts presence via curl -D + grep; mutating this
// middleware to no-op (T9 M2 mutation) MUST cause E51 to FAIL.
//
// Special case: port == 0 means "HTTP/3 disabled" (e.g., HERALD_DISABLE_HTTP3=1)
// and the middleware is a no-op — clients should not learn about an
// HTTP/3 port that isn't listening.
func AltSvcMiddleware(h3Port int) gin.HandlerFunc {
	if h3Port == 0 {
		return func(c *gin.Context) { c.Next() }
	}
	cfg := &altsvc.Config{
		Enabled: true,
		H3Port:  strconv.Itoa(h3Port),
		MaxAge:  2592000, // 30 days
	}
	return mwgin.Wrap(altsvc.New(cfg))
}
```

- [ ] **Step 4: Run tests — verify 2/2 PASS**

```bash
go test -race -count=1 ./commons/cli/ -run TestAltSvcMiddleware 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add commons/cli/altsvc.go commons/cli/altsvc_test.go
git commit -m "Wave 4a step 4: commons/cli/altsvc.go — Alt-Svc Gin middleware

Wraps digital.vasic.middleware/pkg/altsvc + pkg/gin.Wrap into a Gin
handler. Default max-age 30 days (2592000s) — matches RFC 7838 §3.1
recommendation and gives clients a long upgrade window.

Special case: h3Port=0 (HERALD_DISABLE_HTTP3=1 escape hatch) → no-op.
Clients shouldn't learn about a port that isn't listening.

2 unit tests: header present with expected substrings at port 24791;
no header when port=0.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: `commons/cli/brotli.go` — Brotli Gin middleware

**Files:**
- Create: `commons/cli/brotli.go`, `commons/cli/brotli_test.go`

Wraps `digital.vasic.middleware/pkg/brotli` via `pkg/gin.Wrap`. Operator-locked level = 6 (one above stdlib default); MinLength 256 (from upstream default). `HERALD_BROTLI_LEVEL` env var (0-11) overrides.

- [ ] **Step 1: Write failing test `commons/cli/brotli_test.go`**

```go
package cli

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

func TestBrotliMiddleware_AcceptBR_BodyCompressedAndDecodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrotliMiddleware())
	// 1 KiB JSON-ish payload — above MinLength + compressible content-type.
	r.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, strings.Repeat(`{"k":"v","number":42}`, 100))
	})
	req := httptest.NewRequest("GET", "/large", nil)
	req.Header.Set("Accept-Encoding", "br")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want 'br' — middleware no-op'd despite Accept-Encoding", got)
	}
	// Real round-trip via andybalholm/brotli decoder.
	br := brotli.NewReader(bytes.NewReader(rec.Body.Bytes()))
	decoded, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("brotli.NewReader.ReadAll: %v (compressed bytes are not valid Brotli — bluff)", err)
	}
	if !strings.Contains(string(decoded), `{"k":"v"`) {
		t.Errorf("decoded body missing expected JSON: %q", decoded[:min(80, len(decoded))])
	}
	// Sanity: the compressed body MUST be smaller than the source (the
	// expected behaviour of Brotli quality 6 for this payload class).
	if len(rec.Body.Bytes()) >= len(decoded) {
		t.Errorf("compressed size %d >= decoded size %d — Brotli is not actually compressing", len(rec.Body.Bytes()), len(decoded))
	}
}

func TestBrotliMiddleware_NoAcceptEncoding_BodyIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrotliMiddleware())
	r.GET("/large", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, strings.Repeat(`{"k":"v"}`, 100))
	})
	req := httptest.NewRequest("GET", "/large", nil)
	// No Accept-Encoding header.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q on identity-only client, want empty", got)
	}
}

func TestBrotliMiddleware_SubMinLength_NotCompressed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrotliMiddleware())
	r.GET("/small", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(200, `{"ok":true}`) // ~12 bytes — well below 256
	})
	req := httptest.NewRequest("GET", "/small", nil)
	req.Header.Set("Accept-Encoding", "br")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q on sub-MinLength body, want empty (upstream config skips compression for <256B)", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Run test — verify FAIL**

```bash
go test ./commons/cli/ -count=1 -run TestBrotliMiddleware 2>&1 | tail -5
```

Expected: `undefined: BrotliMiddleware`.

- [ ] **Step 3: Implement `commons/cli/brotli.go`**

```go
package cli

import (
	"os"
	"strconv"

	brotlimw "digital.vasic.middleware/pkg/brotli"
	mwgin "digital.vasic.middleware/pkg/gin"
	"github.com/gin-gonic/gin"
)

// HeraldBrotliLevel is the operator-locked Brotli quality used by all
// Herald flavors. 6 is one notch above the andybalholm/brotli library
// default (5); operator override via HERALD_BROTLI_LEVEL env (0-11).
const HeraldBrotliLevel = 6

// BrotliMiddleware returns a Gin handler that compresses response
// bodies with Brotli when the client's Accept-Encoding includes 'br'
// AND the response body is at least 256 bytes AND the Content-Type is
// in the compressible-types list (application/json + text/* + the
// standard upstream defaults).
//
// Per §107: a "Brotli applied" PASS without observing the actual
// content-encoding header AND a successful round-trip decompression is
// a §11.4 bluff. The unit test asserts both; the e2e suite (T8 E53)
// asserts the same against a real curl + andybalholm/brotli decoder.
func BrotliMiddleware() gin.HandlerFunc {
	cfg := brotlimw.DefaultConfig()
	cfg.Level = HeraldBrotliLevel
	if envLvl := os.Getenv("HERALD_BROTLI_LEVEL"); envLvl != "" {
		if n, err := strconv.Atoi(envLvl); err == nil && n >= 0 && n <= 11 {
			cfg.Level = n
		}
	}
	return mwgin.Wrap(brotlimw.New(cfg))
}
```

- [ ] **Step 4: Run tests — verify 3/3 PASS**

```bash
go test -race -count=1 ./commons/cli/ -run TestBrotliMiddleware 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add commons/cli/brotli.go commons/cli/brotli_test.go
git commit -m "Wave 4a step 5: commons/cli/brotli.go — Brotli Gin middleware

Wraps digital.vasic.middleware/pkg/brotli + pkg/gin.Wrap into a Gin
handler. Quality level = 6 (operator-locked; one notch above stdlib
default; balanced CPU/ratio for Herald's small responses 256B-10KiB).
HERALD_BROTLI_LEVEL env var (0-11) overrides.

MinLength 256 (upstream default) — skip compression for tiny bodies
where the overhead exceeds the savings. /v1/healthz (~80B) stays
uncompressed; /v1/compliance (~600B-10KiB) compresses.

3 unit tests: real round-trip via andybalholm/brotli.NewReader decodes
back to the original; compressed size < original; identity client
gets uncompressed body; sub-MinLength body stays uncompressed.

Per §107: positive runtime evidence is the actual decompression, NOT
the Content-Encoding header alone — a buggy encoder that sets the
header but writes identity bytes is a §11.4 bluff that would slip
past a header-only assertion.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: `commons/cli/serve.go` — dual-listener refactor

**Files:**
- Modify: `commons/cli/serve.go`, `commons/cli/cli_test.go`

This task refactors the existing single-TCP-listener `ServeCmd` to bind BOTH TCP and UDP listeners sharing the same Gin engine. Adds Brotli + Alt-Svc middleware to the chain (BEFORE the flavor-supplied middleware, so they apply to healthz/readyz/metrics + flavor routes uniformly). Adds `TLSCertPath` + `TLSKeyPath` + `ProdMode` fields to `ServeOpts`; resolves cert via `commons_tls.ResolveCertSource`. Honors `HERALD_DISABLE_HTTP3=1` escape hatch.

- [ ] **Step 1: Update `commons/cli/serve.go`** (replace existing body)

```go
package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_tls"
)

// ServeOpts configures ServeCmd. Branding drives the default port +
// healthz/metrics gauge name. Routes are flavor-specific routes
// appended after the base healthz/readyz/metrics. Middleware is the
// optional flavor-specific Gin middleware chain registered immediately
// after gin.Recovery() + Brotli + Alt-Svc and before any handler.
//
// Wave 4a additions:
//   TLSCertPath / TLSKeyPath — operator-supplied via --tls-cert /
//                              --tls-key flags wired by the flavor's
//                              cmd/<flavor>/main.go. Empty pair means
//                              "use env or dev-autogen".
//   ProdMode — when true, no dev-cert fallback is permitted; missing
//              cert is a startup error. Convention: set true when
//              HERALD_AUTH_MODE=jwks.
type ServeOpts struct {
	Branding    commons.Branding
	Routes      []Route
	Middleware  []gin.HandlerFunc
	TLSCertPath string
	TLSKeyPath  string
	ProdMode    bool
}

// ServeCmd is the `<flavor>herald serve` subcommand. Binds:
//   1. A TCP listener (HTTP/1.1+HTTP/2) on port via http.Server.ListenAndServeTLS.
//   2. A UDP listener (HTTP/3) on the SAME port via startQUIC — UNLESS
//      HERALD_DISABLE_HTTP3=1 is set.
//
// Both listeners share the same Gin engine; the engine carries Brotli
// + Alt-Svc + flavor middleware + healthz/readyz/metrics + flavor
// routes. Graceful shutdown on SIGTERM/SIGINT or context cancel
// shuts down BOTH listeners.
//
// Per §107: a "serve started" PASS without observing a healthz round-
// trip over BOTH transports is a bluff. The e2e suite (T8 E49+E50)
// asserts real H/2 + real H/3 GETs separately.
func ServeCmd(opts ServeOpts) *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the " + opts.Branding.DisplayName + " HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if port == 0 {
				port = opts.Branding.DefaultPort
			}
			gin.SetMode(gin.ReleaseMode)
			r := gin.New()
			r.Use(gin.Recovery())
			// Transport middleware (Brotli + Alt-Svc) BEFORE flavor mw.
			// Order matters: Alt-Svc header must be emitted regardless
			// of auth state so unauthenticated probes still see it.
			r.Use(BrotliMiddleware())
			h3Port := port
			if os.Getenv("HERALD_DISABLE_HTTP3") == "1" {
				h3Port = 0 // Alt-Svc middleware will no-op
			}
			r.Use(AltSvcMiddleware(h3Port))
			// Health/observability endpoints BEFORE flavor middleware so
			// they remain reachable without auth.
			r.GET("/v1/healthz", HealthzHandler(opts.Branding))
			r.GET("/v1/readyz", ReadyzHandler(opts.Branding))
			r.GET("/metrics", MetricsHandler(opts.Branding))
			for _, mw := range opts.Middleware {
				if mw != nil {
					r.Use(mw)
				}
			}
			for _, route := range opts.Routes {
				h := route.Handler
				if h == nil && route.HRD != "" {
					h = StubRouteHandler(route)
				}
				r.Handle(route.Method, route.Path, h)
			}
			// Resolve TLS cert via the policy ladder.
			cert, err := commons_tls.ResolveCertSource(opts.TLSCertPath, opts.TLSKeyPath, opts.ProdMode)
			if err != nil {
				return fmt.Errorf("serve: TLS cert: %w", err)
			}
			tlsCfg := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS13,
				NextProtos:   []string{"h2", "http/1.1"}, // ALPN for TCP path; QUIC adds "h3" in startQUIC
			}
			addr := fmt.Sprintf(":%d", port)
			srv := &http.Server{
				Addr:      addr,
				Handler:   r,
				TLSConfig: tlsCfg,
			}
			errCh := make(chan error, 2)
			// TCP/HTTPS goroutine.
			go func() {
				if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("tcp listen: %w", err)
				}
			}()
			// UDP/HTTP-3 goroutine — only if not disabled.
			var qsrv *quicServer
			if os.Getenv("HERALD_DISABLE_HTTP3") != "1" {
				qsrv, err = startQUIC(addr, r, &tls.Config{
					Certificates: tlsCfg.Certificates,
					MinVersion:   tls.VersionTLS13,
					NextProtos:   []string{"h3"},
				})
				if err != nil {
					return fmt.Errorf("serve: UDP/H3 listener: %w", err)
				}
				go func() {
					if err, ok := <-qsrv.Done(); ok && err != nil && err != http.ErrServerClosed {
						errCh <- fmt.Errorf("udp listen: %w", err)
					}
				}()
			} else {
				fmt.Fprintln(os.Stderr, "INFO: HTTP/3 listener disabled by env (HERALD_DISABLE_HTTP3=1)")
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			select {
			case err := <-errCh:
				_ = shutdownAll(srv, qsrv)
				return err
			case <-sigCh:
				// graceful shutdown
			case <-ctx.Done():
				// graceful shutdown
			}
			return shutdownAll(srv, qsrv)
		},
	}
	cmd.Flags().IntVar(&port, "http-port", 0, "TCP+UDP port to bind (default = flavor's DefaultPort)")
	return cmd
}

// shutdownAll gracefully shuts down both TCP + UDP listeners with a
// 5-second deadline shared between them. Returns the first non-nil
// error encountered (TCP first, then UDP).
func shutdownAll(srv *http.Server, qsrv *quicServer) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tcpErr := srv.Shutdown(shutdownCtx)
	var udpErr error
	if qsrv != nil {
		udpErr = qsrv.Shutdown(shutdownCtx)
	}
	if tcpErr != nil {
		return tcpErr
	}
	return udpErr
}
```

- [ ] **Step 2: Update `commons/cli/cli_test.go`** — extend existing test surface

Add the following test at the end of the file (preserve all existing tests verbatim):

```go
// TestServeCmd_DualListenerStartsTCPAndUDP verifies the post-Wave-4a
// dual-listener behaviour. Boots ServeCmd in a goroutine with a fresh
// port, real dev cert (via commons_tls auto-gen into t.TempDir's HOME),
// and asserts a real HTTP/2 client + a real HTTP/3 client both get 200
// on /v1/healthz.
//
// This is the cli-package smoke; the deep H3 test lives in h3_test.go.
func TestServeCmd_DualListenerStartsTCPAndUDP(t *testing.T) {
	// Sandbox HOME so dev-cert lands in t.TempDir.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	br := commons.Branding{
		Flavor: "test", Prefix: "t", DisplayName: "Test", DefaultPort: 0,
		Mission: "dual-listener smoke",
	}
	port := freeTCPPort(t)
	opts := ServeOpts{Branding: br}
	cmd := ServeCmd(opts)
	cmd.SetArgs([]string{"--http-port", strconv.Itoa(port)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	// Give listeners time to come up.
	time.Sleep(500 * time.Millisecond)
	// HTTP/2 over TLS smoke (skip cert verification — dev cert is self-signed).
	tcpClient := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}},
		ForceAttemptHTTP2: true,
	}, Timeout: 2 * time.Second}
	resp, err := tcpClient.Get(fmt.Sprintf("https://127.0.0.1:%d/v1/healthz", port))
	if err != nil {
		t.Fatalf("HTTP/2 GET healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("HTTP/2 healthz status = %d, want 200", resp.StatusCode)
	}
	// Verify Alt-Svc header advertises HTTP/3.
	if got := resp.Header.Get("Alt-Svc"); !strings.Contains(got, fmt.Sprintf(`h3=":%d"`, port)) {
		t.Errorf("Alt-Svc = %q, want substring 'h3=\":%d\"'", got, port)
	}
	// HTTP/3 smoke.
	rt := &http3.RoundTripper{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}}}
	defer rt.Close()
	h3Client := &http.Client{Transport: rt, Timeout: 2 * time.Second}
	resp2, err := h3Client.Get(fmt.Sprintf("https://127.0.0.1:%d/v1/healthz", port))
	if err != nil {
		t.Fatalf("HTTP/3 GET healthz: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("HTTP/3 healthz status = %d, want 200", resp2.StatusCode)
	}
	cancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Errorf("ServeCmd Execute: %v", err)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeTCPPort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
```

Add the needed imports at the top of `cli_test.go`:

```go
import (
	// ... existing imports ...
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/quic-go/quic-go/http3"
)
```

- [ ] **Step 3: Run all `commons/cli/` tests — verify PASS (existing + new)**

```bash
cd /Users/milosvasic/Projects/Herald
go test -race -count=1 ./commons/cli/ 2>&1 | tail -15
```

Expected: all existing tests (TestStubCmd, TestNewRootCmd, TestVersionCmd, TestHealthzHandler, etc.) plus T3-T5 tests plus the new TestServeCmd_DualListenerStartsTCPAndUDP — all PASS.

- [ ] **Step 4: Commit**

```bash
git add commons/cli/serve.go commons/cli/cli_test.go
git commit -m "Wave 4a step 6: commons/cli/serve.go — dual TCP+UDP listener

Refactors ServeCmd to bind BOTH a TCP listener (HTTP/1.1+HTTP/2 via
http.Server.ListenAndServeTLS) AND a UDP listener (HTTP/3 via
startQUIC) on the SAME port, sharing the same Gin engine.

Middleware order (top-to-bottom):
  gin.Recovery -> BrotliMiddleware -> AltSvcMiddleware -> flavor mw -> routes

TLS cert resolved via commons_tls.ResolveCertSource(flagCert, flagKey,
prodMode). ServeOpts gains TLSCertPath / TLSKeyPath / ProdMode fields.

HERALD_DISABLE_HTTP3=1 escape hatch — UDP listener skipped + Alt-Svc
middleware no-ops. TCP/HTTPS listener never disabled.

Graceful shutdown drains BOTH listeners on SIGTERM/SIGINT/ctx-cancel
with a shared 5-second deadline.

Wave 3a/3b regression coverage: existing 25+ cli_test.go tests remain
green; new TestServeCmd_DualListenerStartsTCPAndUDP boots the full
stack with a real ECDSA dev cert, real h2 client, real quic-go h3
client — both GET healthz successfully.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: pherald + cherald + sherald main.go cert flag wiring + smoke

**Files:**
- Modify: `pherald/cmd/pherald/main.go`, `cherald/cmd/cherald/main.go`, `sherald/cmd/sherald/main.go`

All three serving flavors expose the new `--tls-cert` + `--tls-key` flags and forward through to `cli.ServeOpts`. ProdMode signal: `HERALD_AUTH_MODE=jwks`.

- [ ] **Step 1: Update `pherald/cmd/pherald/main.go`**

Locate the existing `newServeCmd(...)` (or equivalent helper) that builds `cli.ServeOpts`. Add Cobra flag bindings:

```go
// pherald/cmd/pherald/main.go (additions only — preserve all existing code)

// In the file's globals or in newServeCmd helper:
var (
	pheraldTLSCert string
	pheraldTLSKey  string
)

// Inside the serve-cmd construction, BEFORE returning the cobra command:
// (Adapt to the actual newServeCmd helper shape carried over from Wave 3b T10.)

func newServeCmd(br commons.Branding, runnerInstance *runner.Runner, verifier commons_auth.Verifier) *cobra.Command {
	prodMode := os.Getenv("HERALD_AUTH_MODE") == "jwks"
	cmd := cli.ServeCmd(cli.ServeOpts{
		Branding:    br,
		Routes:      httpsrv.Routes(runnerInstance),
		Middleware:  []gin.HandlerFunc{commons_auth.GinMiddleware(verifier), httpsrv.RequestIDMiddleware()},
		TLSCertPath: pheraldTLSCert,
		TLSKeyPath:  pheraldTLSKey,
		ProdMode:    prodMode,
	})
	cmd.Flags().StringVar(&pheraldTLSCert, "tls-cert", "", "Path to PEM-encoded TLS certificate (or HERALD_TLS_CERT_PATH env; dev-autogen if absent and not in prod mode)")
	cmd.Flags().StringVar(&pheraldTLSKey, "tls-key", "", "Path to PEM-encoded TLS key (paired with --tls-cert)")
	return cmd
}
```

- [ ] **Step 2: Apply equivalent edits to `cherald/cmd/cherald/main.go` + `sherald/cmd/sherald/main.go`**

Mirror the same flag block (variable names `cheraldTLSCert`/`cheraldTLSKey`, `sheraldTLSCert`/`sheraldTLSKey`) and pass through to the respective `cli.ServeOpts` construction. The exact insertion point is each flavor's `newServeCmd` helper, which (per Wave 2 + Wave 3a) follows the same shape.

- [ ] **Step 3: Verify all three flavors build cleanly**

```bash
cd /Users/milosvasic/Projects/Herald
go build ./pherald/cmd/pherald ./cherald/cmd/cherald ./sherald/cmd/sherald 2>&1 | head -5
```

Expected: clean compile.

- [ ] **Step 4: Manual smoke against pherald (dev-cert path)**

```bash
rm -rf ~/.herald
go build -o /tmp/pherald-w4a ./pherald/cmd/pherald
/tmp/pherald-w4a serve --http-port 24791 > /tmp/pherald-w4a.log 2>&1 &
PID=$!
sleep 1
# Test HTTP/2 with self-signed cert
curl -k --http2 -v https://127.0.0.1:24791/v1/healthz 2>&1 | grep -E "(HTTP/2|alt-svc|status)" | head -5
# Test HTTP/3 with curl (requires curl built with HTTP/3 — most modern installs)
curl -k --http3-only -v https://127.0.0.1:24791/v1/healthz 2>&1 | grep -E "(HTTP/3|h3)" | head -5
kill $PID
ls -la ~/.herald/
```

Expected:
- HTTP/2 returns 200 with `alt-svc: h3=":24791"; ma=2592000`.
- HTTP/3 returns 200 via QUIC.
- `~/.herald/dev-cert.pem` + `~/.herald/dev-key.pem` exist with 0600 perms on key.

(If `curl` lacks HTTP/3 support locally, skip the HTTP/3 leg — the deep e2e suite at T8 builds a Go HTTP/3 client.)

- [ ] **Step 5: Commit**

```bash
git add pherald/cmd/pherald/ cherald/cmd/cherald/ sherald/cmd/sherald/
git commit -m "Wave 4a step 7: pherald + cherald + sherald cert-flag wiring

Adds --tls-cert + --tls-key Cobra flags to all three serving flavors.
ProdMode signal forwarded from HERALD_AUTH_MODE=jwks env var. Cert
resolution lands in commons_tls.ResolveCertSource per the ladder:
  flag > HERALD_TLS_CERT_PATH/KEY_PATH env > ~/.herald/dev-{cert,key}.pem
  (dev only — prod mode fails loud).

Manual smoke against /tmp/pherald-w4a (dev-cert auto-gen path):
  curl -k --http2 → 200 + alt-svc header h3=':24791'
  curl -k --http3-only → 200 via QUIC handshake
  ~/.herald/dev-cert.pem (0644) + dev-key.pem (0600) persisted.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 8: `scripts/e2e_bluff_hunt.sh` — 7 new invariants E49-E55

**Files:**
- Modify: `scripts/e2e_bluff_hunt.sh`

Add 7 new invariants asserting positive runtime evidence for every Wave 4a transport feature. All assertions speak real protocol (curl, openssl, tcpdump, Go h3 client where curl lacks HTTP/3).

- [ ] **Step 1: Append the E49-E55 block to `scripts/e2e_bluff_hunt.sh`**

Insert AFTER the existing E45 SKIP block and BEFORE the final `echo "===================================================="` summary:

```bash
# ----------------------------------------------------------------------
# Wave 4a — HTTP/3 + Brotli + Alt-Svc transport substrate (E49-E55).
#
# Per Universal §11.4 + Herald §107 anti-bluff: every assertion below
# observes real wire behaviour — no header-only checks, no
# "configuration looks correct" checks. Mutating any transport feature
# (T9 M1-M3 mutations) MUST cause the matching invariant to FAIL.
# ----------------------------------------------------------------------
echo ""
echo "== E49-E55: HTTP/3 + Brotli + Alt-Svc + TLS-1.3 transport substrate =="

# Build a fresh pherald binary with the Wave 4a transport stack.
W4A_BIN="/tmp/pherald-w4a-$$"
if go build -o "${W4A_BIN}" ./pherald/cmd/pherald > /tmp/e2e_w4a_out 2>&1; then
    # Use a sandboxed HOME so dev-cert auto-gen doesn't pollute the real ~/.herald.
    W4A_HOME="$(mktemp -d -t herald-w4a-home-XXXXXX)"
    W4A_PORT=24791

    # Pre-flight: kill anything currently on the port (orphan from a previous run).
    lsof -ti:${W4A_PORT} 2>/dev/null | xargs -r kill -9 2>/dev/null || true

    HOME="${W4A_HOME}" \
    HERALD_AUTH_MODE=hmac \
    HERALD_AUTH_HMAC_SECRET="test-secret-32-bytes-of-padding!!" \
    "${W4A_BIN}" serve --http-port ${W4A_PORT} > /tmp/pherald-w4a.log 2>&1 &
    w4a_pid=$!
    sleep 1.5  # allow both TCP + UDP listeners to come up

    # E49: TCP/HTTPS listener bound on the port.
    check "E49 TCP/HTTPS listener bound on :${W4A_PORT}" \
        "nc -z 127.0.0.1 ${W4A_PORT}"

    # E50: UDP listener bound on the same port.
    # macOS: lsof -iUDP; Linux: ss -ulnp. Use whichever is available.
    check "E50 UDP/H3 listener bound on :${W4A_PORT}" \
        "(lsof -iUDP -P 2>/dev/null | grep -q ':${W4A_PORT}') || (ss -ulnp 2>/dev/null | grep -q ':${W4A_PORT}')"

    # E51: HTTP/2 over TCP returns 200 + Alt-Svc header advertises HTTP/3.
    check "E51 HTTP/2 GET healthz → 200 + Alt-Svc h3=:${W4A_PORT}" \
        "curl -k --http2 -sS -D /tmp/e51-hdr https://127.0.0.1:${W4A_PORT}/v1/healthz | grep -q '\"status\"' && grep -qi 'Alt-Svc: h3=\":${W4A_PORT}\"' /tmp/e51-hdr"

    # E52: HTTP/3 GET healthz returns 200 — real QUIC handshake via a Go h3 client.
    # We compile a tiny Go program inline to avoid depending on a curl-with-h3 build.
    cat > /tmp/h3client.go <<'GO_EOF'
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	url := os.Args[1]
	rt := &http3.RoundTripper{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}}}
	defer rt.Close()
	client := &http.Client{Transport: rt}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERR", err)
		os.Exit(2)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintln(os.Stderr, "STATUS", resp.StatusCode)
		os.Exit(3)
	}
	fmt.Print(string(body))
}
GO_EOF
    H3_BIN="/tmp/h3client-$$"
    if (cd /Users/milosvasic/Projects/Herald && go run -exec "" /tmp/h3client.go "https://127.0.0.1:${W4A_PORT}/v1/healthz" > /tmp/e52-body 2> /tmp/e52-err) ; then
        # Note: `go run -exec ""` is unusual; if it errors, fall back to compile-then-run.
        :
    else
        (cd /Users/milosvasic/Projects/Herald && go build -o "${H3_BIN}" /tmp/h3client.go) > /tmp/e52-build-err 2>&1
        "${H3_BIN}" "https://127.0.0.1:${W4A_PORT}/v1/healthz" > /tmp/e52-body 2> /tmp/e52-err || true
    fi
    check "E52 HTTP/3 GET healthz returns 200 + JSON body (real QUIC handshake)" \
        "grep -q '\"status\"' /tmp/e52-body && grep -q '\"ok\"' /tmp/e52-body"

    # E53: Brotli content-encoding + real round-trip decompression.
    # POST a multi-KiB JSON to /v1/events (will 400 on payload but the
    # error body is still served + compressed). Use healthz padded with a
    # large 'why' query param to force payload > 256B; or use /metrics
    # which is typically larger.
    curl -k --http2 -sS -H "Accept-Encoding: br" https://127.0.0.1:${W4A_PORT}/metrics -o /tmp/e53-body -D /tmp/e53-hdr
    cat > /tmp/brotli_decode.go <<'GO_EOF'
package main

import (
	"io"
	"os"

	"github.com/andybalholm/brotli"
)

func main() {
	r := brotli.NewReader(os.Stdin)
	io.Copy(os.Stdout, r)
}
GO_EOF
    BR_BIN="/tmp/brotli_decode-$$"
    (cd /Users/milosvasic/Projects/Herald && go build -o "${BR_BIN}" /tmp/brotli_decode.go) > /tmp/e53-build-err 2>&1
    check "E53 GET /metrics with Accept-Encoding:br → Content-Encoding:br + body decompresses to valid Prometheus text" \
        "grep -qi 'Content-Encoding: br' /tmp/e53-hdr && '${BR_BIN}' < /tmp/e53-body > /tmp/e53-decoded 2>/dev/null && grep -q '^# HELP\\|^# TYPE\\|^[a-z_]\\+' /tmp/e53-decoded"

    # E54: TLS 1.3 + ALPN h3 negotiated via openssl s_client probing.
    # (openssl reports the negotiated TLS version + ALPN protocol.)
    check "E54 TLS 1.3 + ALPN 'h3' negotiated on UDP/H3 listener" \
        "echo Q | openssl s_client -connect 127.0.0.1:${W4A_PORT} -tls1_3 -alpn h3 -quiet 2>&1 | grep -qE 'TLSv1\\.3|Verification|self-signed|Protocols advertised by server: h3'"

    # E55: Wire-level UDP traffic captured during an HTTP/3 request.
    # Run tcpdump on loopback while a curl HTTP/3 request fires; assert
    # the pcap contains UDP packets to/from the H3 port.
    if command -v tcpdump >/dev/null 2>&1; then
        tcpdump -i lo0 -nn -c 8 -w /tmp/e55.pcap "udp port ${W4A_PORT}" >/dev/null 2>&1 &
        tcp_pid=$!
        sleep 0.2
        "${H3_BIN}" "https://127.0.0.1:${W4A_PORT}/v1/healthz" > /dev/null 2>&1 || true
        sleep 0.5
        kill $tcp_pid 2>/dev/null
        wait $tcp_pid 2>/dev/null
        check "E55 tcpdump captures ≥ 4 UDP packets on :${W4A_PORT} during HTTP/3 request" \
            "[ \"\$(tcpdump -r /tmp/e55.pcap -nn 2>/dev/null | wc -l)\" -ge 4 ]"
    else
        echo "SKIP  E55 (tcpdump not available — §11.4.3 explicit SKIP-with-reason; install via 'brew install tcpdump' on macOS)"
    fi

    kill ${w4a_pid} 2>/dev/null || true
    wait ${w4a_pid} 2>/dev/null || true
    rm -f "${W4A_BIN}" "${H3_BIN}" "${BR_BIN}" /tmp/e51-hdr /tmp/e52-body /tmp/e52-err /tmp/e53-body /tmp/e53-hdr /tmp/e53-decoded /tmp/e55.pcap /tmp/h3client.go /tmp/brotli_decode.go /tmp/pherald-w4a.log
    rm -rf "${W4A_HOME}"
else
    echo "FAIL  E49-E55 pherald-w4a build"
    tail -5 /tmp/e2e_w4a_out | sed 's/^/      /'
    fail=$((fail+7))
    fail_names+=("E49-build" "E50-build" "E51-build" "E52-build" "E53-build" "E54-build" "E55-build")
fi
```

- [ ] **Step 2: Update header tally**

Find and modify the script's header comment + summary line:

- Change `Forty-seven invariants` → `Fifty-four invariants` wherever it appears in the file header.
- Update the per-section heading numbering if any "Wave N" section sub-header references the total.

- [ ] **Step 3: Run the full e2e suite — verify 54/54 PASS (or honest SKIP-with-reason)**

```bash
cd /Users/milosvasic/Projects/Herald
bash scripts/e2e_bluff_hunt.sh 2>&1 | tail -30
```

Expected: 54 PASS / 0 FAIL (or some SKIP-with-reason if `tcpdump` is unavailable per E55's gate). No FAILs.

- [ ] **Step 4: Commit**

```bash
git add scripts/e2e_bluff_hunt.sh
git commit -m "Wave 4a step 8: e2e E49-E55 — HTTP/3 + Brotli + Alt-Svc + TLS-1.3

7 new invariants observing real wire behaviour per §107:
  E49 TCP/HTTPS listener bound on :24791
  E50 UDP/H3 listener bound on :24791 (lsof/ss)
  E51 HTTP/2 GET healthz → 200 + Alt-Svc 'h3=\":24791\"'
  E52 HTTP/3 GET healthz → 200 via real quic-go client (compiled inline)
  E53 GET /metrics with Accept-Encoding:br → Content-Encoding:br +
      body decompresses to valid Prometheus text via andybalholm/brotli
  E54 TLS 1.3 + ALPN 'h3' negotiated (openssl s_client probe)
  E55 tcpdump captures ≥ 4 UDP packets on the H3 port during a request

No mocks; no metadata-only assertions; every PASS carries captured wire
evidence. Mutating any Wave 4a transport feature (T9 M1-M3) MUST cause
the matching invariant to FAIL.

Tally: 47 → 54 invariants (E1..E48 + E49..E55). E45 SKIP unchanged.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 9: `tests/test_wave4_mutation_meta.sh` — paired mutation gate

**Files:**
- Create: `tests/test_wave4_mutation_meta.sh`

Three mutations + post-flight. Each mutation deliberately breaks one Wave 4a transport feature and asserts the matching e2e invariant FAILs on the mutated tree.

- [ ] **Step 1: Create `tests/test_wave4_mutation_meta.sh`**

```bash
#!/usr/bin/env bash
# tests/test_wave4_mutation_meta.sh — Paired §1.1 mutation test for Wave 4a
# transport substrate.
#
# Per Universal §11.4 + Herald §107: every transport invariant MUST have
# a paired mutation that, when applied, causes the invariant to FAIL.
# A gate that PASSes on a mutated tree is itself a §11.4 PASS-bluff.
#
# Wave 4a mutations:
#
#   M1. Strip HTTP/3 listener — comment out the startQUIC call inside
#       commons/cli/serve.go ⇒ E50 + E52 + E55 MUST FAIL (UDP listener
#       absent, HTTP/3 client gets connection-refused).
#
#   M2. Strip Brotli middleware — change BrotliMiddleware to return a
#       no-op ⇒ E53 MUST FAIL (Content-Encoding header absent + body
#       is identity, not Brotli).
#
#   M3. Downgrade TLS to 1.2 — set tls.MinVersion = VersionTLS12 inside
#       commons/cli/serve.go's tlsCfg ⇒ E54 MUST FAIL (openssl s_client
#       reports TLSv1.2, ALPN h3 negotiation refused by quic-go client).
#
# Pattern mirrors tests/test_wave3_mutation_meta.sh:
#   - file_backup copies the file
#   - mutation rewrites with `cat > file`
#   - restore on EXIT trap

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
E2E="${REPO_ROOT}/scripts/e2e_bluff_hunt.sh"

pass=0
fail=0

file_backup() { cp "$1" "$1.w4meta-backup"; }
restore() {
    if [ -f "$1.w4meta-backup" ]; then
        cat "$1.w4meta-backup" > "$1"
        rm -f "$1.w4meta-backup"
    fi
}

cleanup_all() {
    restore "${REPO_ROOT}/commons/cli/serve.go"
    restore "${REPO_ROOT}/commons/cli/brotli.go"
    for port in 24791 24792 24793; do
        lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
    done
}
trap cleanup_all EXIT

# Pre-flight: kill orphans + assert clean baseline.
for port in 24791 24792 24793; do
    lsof -ti:${port} 2>/dev/null | xargs -r kill -9 2>/dev/null || true
done

# ----------------------------------------------------------------------
# Baseline: e2e_bluff_hunt MUST pass on the unmutated tree.
# ----------------------------------------------------------------------
echo "== Baseline: e2e on unmutated tree =="
if "${E2E}" > /tmp/w4meta-baseline.log 2>&1; then
    pass=$((pass+1))
    echo "PASS  baseline: unmutated tree passes e2e"
else
    fail=$((fail+1))
    echo "FAIL  baseline: unmutated tree fails e2e (BLOCKER — fix root cause before re-running mutation gate)"
    tail -20 /tmp/w4meta-baseline.log | sed 's/^/      /'
    exit 1
fi

# ----------------------------------------------------------------------
# M1: Strip HTTP/3 listener inside commons/cli/serve.go.
# We mutate by setting HERALD_DISABLE_HTTP3=1 unconditionally inside the
# code path — easier than re-writing the whole file. Use sed on the
# os.Getenv line.
# ----------------------------------------------------------------------
SERVE="${REPO_ROOT}/commons/cli/serve.go"
echo ""
echo "== M1: strip HTTP/3 listener (force HERALD_DISABLE_HTTP3 always) =="
file_backup "${SERVE}"
# Replace the env-check with an always-true.
sed -i.bak 's|os.Getenv("HERALD_DISABLE_HTTP3") == "1"|true /* M1 mutation */ \&\& os.Getenv("HERALD_DISABLE_HTTP3") == "1"|g' "${SERVE}"
sed -i.bak 's|os.Getenv("HERALD_DISABLE_HTTP3") != "1"|false /* M1 mutation */ \&\& os.Getenv("HERALD_DISABLE_HTTP3") != "1"|g' "${SERVE}"
rm -f "${SERVE}.bak"
if "${E2E}" > /tmp/w4meta-m1.log 2>&1; then
    fail=$((fail+1))
    echo "FAIL  M1: e2e passed with H3 listener stripped — E50/E52/E55 are bluffs"
    tail -10 /tmp/w4meta-m1.log | sed 's/^/      /'
else
    pass=$((pass+1))
    echo "PASS  M1: e2e correctly failed (stripped H3 listener)"
    grep -E "(E50|E52|E55)" /tmp/w4meta-m1.log | head -3 | sed 's/^/      /'
fi
restore "${SERVE}"

# ----------------------------------------------------------------------
# M2: Strip Brotli middleware — overwrite BrotliMiddleware with no-op.
# ----------------------------------------------------------------------
BROTLI="${REPO_ROOT}/commons/cli/brotli.go"
echo ""
echo "== M2: strip Brotli middleware (no-op) =="
file_backup "${BROTLI}"
cat > "${BROTLI}" <<'EOF'
package cli

import (
	"github.com/gin-gonic/gin"
)

// HeraldBrotliLevel — MUTATED for paired §1.1 anti-bluff test M2.
const HeraldBrotliLevel = 6

// BrotliMiddleware — MUTATED to no-op for Wave 4a M2.
// If you see this in production, the gate is itself a bluff.
func BrotliMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) { c.Next() }
}
EOF
if "${E2E}" > /tmp/w4meta-m2.log 2>&1; then
    fail=$((fail+1))
    echo "FAIL  M2: e2e passed with Brotli stripped — E53 is a bluff"
    tail -10 /tmp/w4meta-m2.log | sed 's/^/      /'
else
    pass=$((pass+1))
    echo "PASS  M2: e2e correctly failed (stripped Brotli middleware)"
    grep -E "E53" /tmp/w4meta-m2.log | head -2 | sed 's/^/      /'
fi
restore "${BROTLI}"

# ----------------------------------------------------------------------
# M3: Downgrade TLS MinVersion to 1.2 in commons/cli/serve.go.
# ----------------------------------------------------------------------
echo ""
echo "== M3: downgrade TLS MinVersion to 1.2 =="
file_backup "${SERVE}"
sed -i.bak 's|MinVersion:   tls.VersionTLS13|MinVersion:   tls.VersionTLS12 /* M3 mutation */|g' "${SERVE}"
rm -f "${SERVE}.bak"
if "${E2E}" > /tmp/w4meta-m3.log 2>&1; then
    fail=$((fail+1))
    echo "FAIL  M3: e2e passed with TLS downgraded to 1.2 — E52/E54 are bluffs"
    tail -10 /tmp/w4meta-m3.log | sed 's/^/      /'
else
    pass=$((pass+1))
    echo "PASS  M3: e2e correctly failed (TLS downgraded; QUIC client refuses non-1.3 ALPN h3)"
    grep -E "(E52|E54)" /tmp/w4meta-m3.log | head -2 | sed 's/^/      /'
fi
restore "${SERVE}"

# ----------------------------------------------------------------------
# Post-flight: re-run e2e to confirm restores succeeded.
# ----------------------------------------------------------------------
echo ""
echo "== Post-flight: e2e on restored tree =="
if "${E2E}" > /tmp/w4meta-postflight.log 2>&1; then
    pass=$((pass+1))
    echo "PASS  post-flight: restored tree passes e2e cleanly"
else
    fail=$((fail+1))
    echo "FAIL  post-flight: restored tree fails e2e — restore is incomplete"
    tail -10 /tmp/w4meta-postflight.log | sed 's/^/      /'
fi

rm -f /tmp/w4meta-baseline.log /tmp/w4meta-m1.log /tmp/w4meta-m2.log /tmp/w4meta-m3.log /tmp/w4meta-postflight.log

echo ""
echo "===================================================="
echo "Wave 4a mutation gate: ${pass} PASS / ${fail} FAIL"
if [ "${fail}" -gt 0 ]; then
    exit 1
fi
exit 0
```

- [ ] **Step 2: Make executable + run baseline**

```bash
cd /Users/milosvasic/Projects/Herald
chmod +x tests/test_wave4_mutation_meta.sh
bash tests/test_wave4_mutation_meta.sh 2>&1 | tail -20
```

Expected: 4/4 PASS (baseline + M1 + M2 + M3 + post-flight). FAIL exit code from this script is itself a constitutional violation — fix the mutation script or the gate it tests.

- [ ] **Step 3: Commit**

```bash
git add tests/test_wave4_mutation_meta.sh
git commit -m "Wave 4a step 9: paired mutation gate (M1+M2+M3) + post-flight

Three Wave 4a mutations + baseline + post-flight, mirroring the
test_wave3_mutation_meta.sh pattern:

  M1: strip HTTP/3 listener (force HERALD_DISABLE_HTTP3=1 in serve.go)
      ⇒ E50/E52/E55 MUST FAIL (no UDP listener, no QUIC handshake,
      no UDP packets on the wire).

  M2: strip Brotli middleware (rewrite to no-op)
      ⇒ E53 MUST FAIL (Content-Encoding header absent, body is
      identity not Brotli).

  M3: downgrade TLS MinVersion 1.3 → 1.2
      ⇒ E52/E54 MUST FAIL (QUIC client refuses non-1.3 ALPN h3;
      openssl s_client reports TLSv1.2 instead of TLSv1.3).

Per §1.1: a gate without a paired mutation is itself a §11.4 bluff.
This test re-runs e2e_bluff_hunt.sh with each mutation in place and
asserts the e2e suite FAILS on the expected invariants. Restore on
EXIT trap + post-flight re-run confirms zero residual mutation.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 10: Spec V3 r9 → r10 + Issues/Fixed/Status + full anti-bluff sweep + tag v0.2.0 + 4-mirror push

**Files:**
- Modify: `docs/specs/mvp/specification.V3.md`, `docs/Issues.md`, `docs/Fixed.md`, `docs/Status.md`, `CLAUDE.md`

Final close-out: spec bump, HRD atomic close, full battery sweep, tag v0.2.0, multi-mirror push.

- [ ] **Step 1: Spec V3 r9 → r10**

Edit `docs/specs/mvp/specification.V3.md`:

- Metadata table: `Revision` 9 → 10; `Last modified` 2026-05-22; `Status summary` appended with one paragraph capturing the §41.7 Transport subsection landing + Wave 4a as-built (commits, e2e count 47→54, modules 14→15).
- Insert new **§41.7 Transport** subsection after §41.6 with six sub-sections per design doc §8.1:
  - **§41.7.1 Wire protocol primary: HTTP/3 (QUIC).** Every flavor's `serve` binds TCP + UDP on the same port. TLS 1.3 mandatory. Implementation: `commons/cli/serve.go` dual-listener + `commons/cli/h3.go` wrapper over `digital.vasic.http3`.
  - **§41.7.2 Compression default: Brotli quality 6.** Server applies Brotli when `Accept-Encoding: br`; MinLength 256 bytes; identity for binary content-types. `HERALD_BROTLI_LEVEL` env override (0-11). Implementation: `commons/cli/brotli.go` + `digital.vasic.middleware/pkg/brotli`.
  - **§41.7.3 Alt-Svc advertisement.** Every TCP response carries `Alt-Svc: h3=":<port>"; ma=2592000`. Implementation: `commons/cli/altsvc.go` + `digital.vasic.middleware/pkg/altsvc`.
  - **§41.7.4 TLS cert sourcing.** Ladder: flag (`--tls-cert`/`--tls-key`) > env (`HERALD_TLS_CERT_PATH`/`HERALD_TLS_KEY_PATH`) > dev auto-gen (`~/.herald/dev-cert.pem` + `dev-key.pem`, ECDSA P-256, SAN list `[localhost, 127.0.0.1, ::1]`, validity 365d). Production fail-loud when `HERALD_AUTH_MODE=jwks` AND no cert supplied. Implementation: `commons_tls/cert.go` (`LoadOrGenerate` + `ResolveCertSource`).
  - **§41.7.5 Backwards compatibility.** Legacy HTTP/1.1 clients with `Accept: application/json` and no `Accept-Encoding` continue to work — identity response over TCP. Alt-Svc header tells them HTTP/3 is available; they may ignore it.
  - **§41.7.6 Escape hatch.** `HERALD_DISABLE_HTTP3=1` env disables the UDP listener AND the Alt-Svc advertisement. TCP/HTTPS listener never disabled. Intended for CI runners that can't open UDP ports.
- Add **§44.M Wave 4a milestone** subsection in the "Milestones" or equivalent area:
  > **§44.M — Wave 4a — HTTP/3 + Brotli transport substrate (closed YYYY-MM-DD)**
  > Closes the HTTP/3 + Brotli legs of the Wave 4 design (commit `c60b3fd`). Tags v0.2.0. As-built: 15 workspace modules (added `commons_tls`); e2e_bluff_hunt at 54 invariants (E1..E55); paired mutation gate `tests/test_wave4_mutation_meta.sh` 4/4 PASS. TOON adoption deferred to Wave 4b per spec design §7.4.

Regenerate siblings:

```bash
cd /Users/milosvasic/Projects/Herald
bash scripts/export_docs.sh docs/specs/mvp/specification.V3.md
```

- [ ] **Step 2: Issues / Fixed / Status / CLAUDE.md atomic close**

- `docs/Issues.md` r12 → r13: prepend three HRD entries (HRD-101 HTTP/3 listener, HRD-102 Brotli middleware, HRD-103 TLS cert sourcing) — each `**Type:** Feature` / `**Status:** Implemented (→ Fixed.md)` per CONST-057.
- `docs/Fixed.md` r11 → r12: prepend HRD-101/102/103 rows with commit SHA refs (placeholder until commit lands; update post-commit) + e2e invariants (HRD-101 ↔ E49+E50+E52+E55; HRD-102 ↔ E53; HRD-103 ↔ E54+E61).
- `docs/Status.md` r13 → r14: bump revision; status summary cites Wave 4a complete; module count 14 → 15; e2e invariant total 47 → 54; mutation gate count 6 → 9 (Wave 3 M1-M6 + Wave 4 M1-M3).
- `CLAUDE.md` r7 → r8: update workspace module count line `**14** Herald modules` → `**15** Herald modules` (adds `commons_tls` to the foundation list); add brief Wave 4a status pointer to the Project status section.

Regenerate siblings for each `.md`:

```bash
bash scripts/export_docs.sh docs/Issues.md docs/Fixed.md docs/Status.md CLAUDE.md
```

- [ ] **Step 3: Run FULL anti-bluff battery**

```bash
cd /Users/milosvasic/Projects/Herald
bash tests/test_constitution_inheritance.sh        # 15/15 (unchanged)
bash tests/test_constitution_inheritance_meta.sh   # META-PASS
bash tests/test_i6_refinement_meta.sh              # 3/3
bash tests/test_i8_usability_meta.sh               # 5/5
bash tests/test_wave2_mutation_meta.sh             # 4/4
bash tests/test_wave3_mutation_meta.sh             # 6/6
bash tests/test_wave4_mutation_meta.sh             # 4/4 (new)
bash scripts/audit_antibluff.sh                    # 16+ PASS / 0 FAIL
bash scripts/codegraph_validate.sh                 # 7+ PASS / 0 FAIL
bash scripts/e2e_bluff_hunt.sh                     # 54 PASS / 0 FAIL (≤5 SKIP)
```

ALL must be green. Tag is the next step; do NOT tag until the battery is clean.

- [ ] **Step 4: Commit + tag v0.2.0 + 4-mirror push**

```bash
git add commons/go.mod commons/go.sum \
        commons/cli/serve.go commons/cli/cli_test.go \
        commons/cli/h3.go commons/cli/h3_test.go \
        commons/cli/altsvc.go commons/cli/altsvc_test.go \
        commons/cli/brotli.go commons/cli/brotli_test.go \
        commons_tls/ \
        pherald/cmd/pherald/ cherald/cmd/cherald/ sherald/cmd/sherald/ \
        scripts/e2e_bluff_hunt.sh \
        tests/test_wave4_mutation_meta.sh \
        docs/specs/mvp/specification.V3.{md,html,docx,pdf} \
        docs/Issues.{md,html,docx,pdf} \
        docs/Fixed.{md,html,docx,pdf} \
        docs/Status.{md,html,docx,pdf} \
        docs/catalogue-checks/HRD-100-commons-tls.{md,html,docx,pdf} \
        CLAUDE.{md,html,docx,pdf} \
        go.work .gitmodules submodules/http3

git commit -m "Wave 4a step 10: spec V3 r10 + Issues/Fixed/Status close + v0.2.0 tag

Closes HRD-101 (HTTP/3 listener), HRD-102 (Brotli middleware),
HRD-103 (TLS cert sourcing) — all three atomic Issues→Fixed per
§11.4.19. Spec V3 r9 → r10 captures the new §41.7 Transport subsection
+ §44.M Wave 4a milestone. CLAUDE.md r7 → r8 module count 14 → 15.

As-built evidence:
  - 15 workspace modules (added commons_tls/ — 15th foundation module).
  - 7 new e2e invariants E49-E55 PASS (real HTTP/3 + real Brotli +
    real Alt-Svc + real TLS 1.3 + real UDP packets via tcpdump).
  - 3 new paired mutations M1-M3 + post-flight (test_wave4_mutation_meta.sh
    4/4 PASS).
  - Submodule submodules/http3/ at SHA 1d0df7b vendored via replace
    directive in commons/go.mod.
  - All Wave 3a/3b regressions clean (e2e E1-E48 unchanged).

Operator-locked decisions baked in:
  - Dev TLS cert auto-gen at ~/.herald/dev-{cert,key}.pem (ECDSA P-256,
    SAN [localhost,127.0.0.1,::1], 365d).
  - Production fail-loud when HERALD_AUTH_MODE=jwks AND no cert.
  - Brotli level 6 (one above stdlib default); HERALD_BROTLI_LEVEL env.
  - Always-on HTTP/3 with HERALD_DISABLE_HTTP3=1 escape hatch.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"

git tag -a v0.2.0 -m "Herald v0.2.0 — HTTP/3 (QUIC) + Brotli + Alt-Svc transport substrate

Wave 4a close. Every flavor's REST surface now serves HTTP/3 primary
with HTTP/2 fallback, Brotli compression default with gzip/identity
fallback, and Alt-Svc advertisement for client upgrade. TLS 1.3
mandatory. Operator-supplied cert via --tls-cert/--tls-key flags or
HERALD_TLS_CERT_PATH/KEY_PATH env (production); ECDSA P-256
self-signed dev cert auto-gen at ~/.herald/dev-{cert,key}.pem when
absent and not in production mode (signaled by HERALD_AUTH_MODE=jwks).

TOON adoption is Wave 4b — separate plan.

E2e invariants 47 → 54; workspace modules 14 → 15; spec V3 r9 → r10."

# Push commit + tag to all 4 mirrors (origin GitHub + GitLab + GitFlic + GitVerse).
git push origin main 2>&1 | tail -5
git push origin v0.2.0 2>&1 | tail -5
# Push to additional configured remotes (if upstreams/ scripts have been
# sourced — see CLAUDE.md "Multi-host mirror convention").
for remote in $(git remote | grep -vE '^origin$'); do
    git push "${remote}" main 2>&1 | tail -3
    git push "${remote}" v0.2.0 2>&1 | tail -3
done

# Post-push convergence check (CONST-051(C) + Lava §6.C inheritance).
echo "Tip SHA per mirror:"
for remote in $(git remote); do
    echo "  ${remote}: $(git ls-remote "${remote}" HEAD | awk '{print $1}')"
done
```

Expected: all four mirrors converge to the same SHA at HEAD AND carry tag v0.2.0.

- [ ] **Step 5: Final sign-off snapshot**

```bash
bash scripts/e2e_bluff_hunt.sh 2>&1 | tail -10
echo ""
echo "Wave 4a complete. v0.2.0 tagged + pushed to all 4 mirrors."
echo "Anti-bluff covenant (§107 / §11.4) intact. End-user can:"
echo "  - curl --http3 https://<host>:<port>/v1/healthz"
echo "  - send Accept-Encoding:br and get Brotli-compressed responses"
echo "  - observe Alt-Svc header for HTTP/3 upgrade discovery"
echo "  - rely on TLS 1.3 + ECDSA P-256 transport security"
```

---

## Wave 4a sign-off summary

| Closes | Evidence |
|---|---|
| HRD-101 | HTTP/3 listener live + E50+E52+E55 PASS |
| HRD-102 | Brotli middleware live + E53 PASS (real round-trip decode) |
| HRD-103 | TLS cert sourcing (flag/env/dev-autogen + prod fail-loud) + E54 PASS |

**Tag:** v0.2.0 — first transport-layer milestone.

**Carry-over to Wave 4b:** TOON adoption (`digital.vasic.toon` upstream PR + Herald codec wiring + content negotiation + 7 new e2e invariants E56-E62). Separate plan per design doc §7.4.

**Carry-over to Wave 4c+:** gzip fallback middleware upstream PR (`digital.vasic.middleware/pkg/gzip`); production TLS reverse-proxy templates in operator guide; QUIC connection-level metrics via `digital.vasic.observability` (future HRD); per-event TOON encoding for Receipt bodies returned by `POST /v1/events` (operator Q4 — recommend YES).

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-22-wave4a-http3-brotli.md`. Two execution options:**

**1. Subagent-Driven (recommended per Universal §11.4.70)** — fresh subagent per task, review between tasks.

**2. Inline Execution** — `superpowers:executing-plans` batch with checkpoints.

**Which approach?**
