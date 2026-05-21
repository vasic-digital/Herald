<div align="center">

![Herald](../../assets/logo/herald_logo_square_128.png){width=96px height=96px}

</div>

# Catalogue-Check — HRD-099 commons_auth/

> **Renumbered 2026-05-21**: Originally tracked as HRD-093 during Wave 3a design + implementation; renumbered to HRD-099 in the post-Wave-3a doc-cleanup pass to resolve a collision with Wave 2's HRD-093 (sherald flavor scaffold, commit `6145cec`). All Wave 3a commits (`dbbea95..0cc6fad`) still reference HRD-093 in their messages — that's the executing-time history; HRD-099 is the canonical anchor going forward.

| Field | Value |
|---|---|
| Date | 2026-05-21 |
| Target | `commons_auth/` (JWT verifier + Gin middleware) |
| Orgs queried | `vasic-digital/*`, `HelixDevelopment/*` |
| Verdict | **no-match → vendor as Herald-internal package** |
| Evidence commits | Wave 3a task 1 (commit `dbbea95`) |

## Search performed

1. `gh search repos --owner=vasic-digital --owner=HelixDevelopment 'jwt jwks gin' --limit 30` → 0 hits with JWT+JWKS+Gin middleware shape.
2. Reviewed `digital.vasic.auth` (`submodules/auth/pkg/jwt/jwt.go`) — provides a single-secret HS-only `Manager` with `Create`/`Validate`/`Refresh` methods. NOT a JWT verification middleware. No JWKS, no Redis cache, no Gin handler, no rotation-race re-fetch.

## Verdict rationale

No existing module in our orgs provides the exact triple Herald needs:

- HS256 AND RS256/ES256 verification in a single `Verifier` interface
- JWKS HTTPS fetch with Redis cache + in-memory fallback + rotation-race re-fetch
- Gin middleware factory that stores claims into request context under a documented key

`digital.vasic.auth/pkg/jwt` covers ~30% of (1) only; (2) and (3) are unimplemented. Forking + extending would mix Herald-specific opinions (Redis cache key prefix `herald:auth:jwks:`, claim-shape `tenant`+`sub`) into a project-neutral submodule — which CONST-051(B) forbids. Vendoring as Herald-internal `commons_auth/` is the correct choice.

When the Herald project later identifies primitives general enough to lift back, we can promote them upstream per §11.4.35 with an explicit `Lifted from herald to digital.vasic.auth per §11.4.35` commit annotation.

## Public surface

See `commons_auth/verifier.go` Verifier interface + Config struct, `commons_auth/middleware.go` GinMiddleware, `commons_auth/claims.go` TenantFromClaims/SubjectFromClaims helpers.
