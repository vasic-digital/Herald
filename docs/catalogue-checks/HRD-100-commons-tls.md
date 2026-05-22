<div align="center">

![Herald](../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Catalogue-Check — HRD-100 commons_tls/

| Field | Value |
|---|---|
| Date | 2026-05-22 |
| Target | `commons_tls/` (TLS cert sourcing for HTTP/3 + HTTP/2 dual-listener) |
| Orgs queried | `vasic-digital/*`, `HelixDevelopment/*` |
| Verdict | **no-match → vendor as Herald-internal package** |
| Evidence commits | Wave 4a task 2 |

## Search performed

1. `gh search repos --owner=vasic-digital --owner=HelixDevelopment 'tls cert ecdsa autogen'` → 0 hits exposing the cert-auto-generation + SAN-list shape Herald needs.
2. Reviewed `digital.vasic.http3/internal/testcert` (newly-vendored at submodules/http3, Wave 4a T1). It is:
   - **internal** package — not importable from outside `digital.vasic.http3`.
   - **RSA 2048** — not ECDSA P-256 (HTTP/3 spec strongly prefers EC for smaller wire / faster handshake).
   - **lifetime 1 hour** — designed for unit-test ephemerality, not the operator dev workflow (Herald's "start serve, hack for a week" cycle).
   - **no persistence** — regenerates every call; the operator's browser would see a new self-signed CA on every restart, requiring re-trust each time.
   - **single SAN ("localhost")** — Herald needs `localhost` + `127.0.0.1` + `::1` to cover dual-stack loopback testing.
3. Reviewed `digital.vasic.middleware` (Wave 3a vendor) — owns Gin middleware (Brotli, Alt-Svc, …) but nothing TLS-related.
4. Reviewed `submodules/auth` (`digital.vasic.auth`) — owns JWT verification (HS-only Manager); nothing TLS-related.
5. `crypto/tls` + `crypto/ecdsa` + `crypto/x509` (Go stdlib) provide all primitives; the value-add is the Herald-specific resolution policy:
   - `HERALD_AUTH_MODE=jwks` ⇒ production signal ⇒ fail-loud when no cert/key supplied (no silent dev-autogen fallback in production).
   - Otherwise: flag > env > dev-autogen at `~/.herald/dev-{cert,key}.pem`.
   - Asymmetric configuration guard: exactly one of cert/key present on disk ⇒ refuse to auto-pair operator-supplied material with auto-generated material (almost certainly an operator typo).

## Verdict rationale

No existing module in our orgs provides the specific Herald shape required for the §41.7 dual-listener substrate:

- **ECDSA P-256 self-signed** with `SignatureAlgorithm = ECDSAWithSHA256` (HTTP/3 spec preference; smaller wire / faster handshake than RSA).
- **SAN list** `[localhost, 127.0.0.1, ::1]` — covers loopback dev testing on dual-stack hosts where IPv6 is enabled.
- **Validity 365 days** — long enough for active dev cycles; renew by deleting `~/.herald/dev-{cert,key}.pem` and restarting the flavor.
- **Persisted to `~/.herald/`** — survives Herald restarts; deletable to force regeneration; browser trust survives across restarts.
- **Chmod 0600 on the key file** — Herald's secret-material handling baseline; double-enforced by `os.OpenFile(0o600)` AND a defensive `os.Chmod(0o600)` after write in case umask widens.
- **Resolution policy ladder**: flag > env > dev-autogen (with prod-mode fail-loud override). Mismatched pairs at any tier return an error (refuses silent fall-through when the operator clearly intended to supply something but typo'd one half of the pair).

`digital.vasic.http3/internal/testcert` covers ~25% of (1) only; (2), (3), (4), (5), (6) are unimplemented. Promoting + extending it to fit Herald's full shape would mix Herald-specific opinions (the `~/.herald/` path, the `HERALD_AUTH_MODE` check, the policy ladder) into a project-neutral submodule — CONST-051(B) violation. Vendoring as Herald-internal `commons_tls/` is the correct choice.

When Herald later identifies primitives general enough to lift back (e.g., the bare ECDSA P-256 generator with configurable SAN list and validity), we promote them upstream per §11.4.35 with the explicit `Lifted from herald to digital.vasic.http3 per §11.4.35` commit annotation.

## Public surface

See `commons_tls/cert.go`:

- `Config{CertPath, KeyPath, ProdMode}` — input to `ResolveCertSource`.
- `CertSource{Cert *tls.Certificate, Source string}` — return value; `Source` ∈ {`"flag"`, `"env"`, `"autogen-dev"`}.
- `LoadOrGenerate(certPath, keyPath string) (*tls.Certificate, error)` — load if both files exist; ECDSA P-256 self-signed auto-gen (SAN list, 365-day validity, chmod 0600 on key) if neither exists; error if exactly one exists (asymmetric-config guard).
- `ResolveCertSource(cfg Config) (*CertSource, error)` — Herald's full policy ladder.

## §107 anti-bluff covenant

A "TLS configured" PASS without observing the actual material on disk + the actual error path firing in production mode is a §11.4 bluff. The unit tests at `commons_tls/cert_test.go` discharge this covenant:

- `TestLoadOrGenerate_NeitherExists_AutoGenerates` parses the generated `x509.Certificate`, asserts `SignatureAlgorithm == ECDSAWithSHA256`, asserts curve `P-256`, asserts SAN DNS contains `localhost`, asserts SAN IPs contain `127.0.0.1` AND `::1`, asserts key-file `Mode().Perm() == 0o600`. A test that only `Stat`-checked the file would PASS even on garbage content.
- `TestResolveCertSource_ProdModeNoFlags_Errors` observes the actual returned error string and asserts `Contains(err, "production mode")` — proves the prod-mode branch actually fired (not an unrelated earlier validation error that happens to also fail).
- `TestResolveCertSource_FlagWinsOverEnv` cross-checks the resolved cert bytes byte-for-byte against the flag-supplied file — proves the precedence is real, not just label-correct.

E49–E55 (Wave 4a T8) will provide the live-handshake e2e evidence via `openssl s_client` / `curl --http3` against a running pherald.
