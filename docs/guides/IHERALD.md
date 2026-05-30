<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `iherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `iherald` (Incident Herald) — a serving flavor (DefaultPort=24794). Documents `serve` and, honestly, that its only application route `POST /v1/webhooks/page` is currently a **501 stub** (HRD-024 pending). ANTI-BLUFF: derived from the built `iherald` binary (`iherald --help`, `version --json`) + `commons/branding.go` + `iherald/internal/http/routes.go`. Per the §11.4.69 anti-bluff posture, iherald serves an honest 501 + HRD pointer rather than a fake 200 stub — this guide states that plainly. |
| Issues | (none specific to this guide) |
| Continuation | Bump when HRD-024 lands the live `/v1/webhooks/page` paging handler body. |

## Table of contents

- [§1. What `iherald` is](#1-what-iherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. `serve` — and the 501 paging stub](#4-serve--and-the-501-paging-stub)
- [§5. References](#5-references)

---

## §1. What `iherald` is

`iherald` is **Incident Herald** — flavor key `i`, prefix `IHR`, default serving port **24794**. Per `commons/branding.go` its mission is "Credential-leak page-out + operator-blocked escalation". It is intended to be the paging surface for incident escalation. As of this revision its HTTP plane is wired but the paging route itself is a **501 stub** awaiting HRD-024 — `iherald` is the most honest demonstration of Herald's anti-bluff posture: it returns a real `501 Not Implemented` + an HRD pointer rather than a fake `200`.

Build it:

```bash
go build -o /tmp/iherald ./iherald/cmd/iherald
```

## §2. The subcommand surface

Verbatim from `iherald --help` — `iherald` has the smallest surface of all flavors (no §43 command catalogue yet):

| Subcommand | What it does |
|---|---|
| `serve` | Start the Incident Herald HTTP server |
| `version` | Print Incident Herald version + build info |
| `completion` | Generate shell autocompletion (Cobra built-in) |

## §3. `version`

```bash
$ iherald version --json
{"arch":"arm64","binary":"iherald","build_time":"unknown","commit":"unknown","flavor":"i","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. `serve` — and the 501 paging stub

`iherald serve` starts the Incident Herald HTTP server on port 24794. Like the other serving flavors it exposes the standard health/metrics surface (`/v1/healthz`, `/v1/readyz`, `/metrics`), and shares the `commons/cli` serve scaffold.

**The one application route, `POST /v1/webhooks/page`, is a 501 stub.** Per `iherald/internal/http/routes.go` the route is registered with `HRD: "HRD-024"` and returns an honest `501 Not Implemented` + an HRD pointer. The package comment states the rationale directly: an honest 501 + HRD pointer beats a 200 stub under the §11.4.69 anti-bluff posture. The live paging integration (the `/v1/webhooks/page` handler body + the §43 escalation command bodies) is scope-locked to the HRD-024 follow-ups.

```bash
iherald serve --http-port 24794
# POST /v1/webhooks/page  →  501 Not Implemented  (HRD-024 pending)
```

Do not build a pager integration against `/v1/webhooks/page` yet — it does not deliver. The `iherald/internal/bindings/` package supplies the constitution-binding scaffold (incident/escalation Evaluator) that the future handler will consume, but the handler body itself is not written.

## §5. References

- Source: `iherald/cmd/iherald/main.go`, `iherald/internal/http/routes.go`, `iherald/internal/bindings/bindings.go`.
- Branding: `commons/branding.go` (flavor `i`, DefaultPort=24794).
- Integration: `docs/INTEGRATION.md` §1/§10 (iherald row — `POST /v1/webhooks/page` 501 stub, HRD-024 pending).
- Open work: HRD-024 (the live paging handler + §43 escalation command bodies).
- Companion flavor guides: `docs/guides/{PHERALD,SHERALD,CHERALD,BHERALD,RHERALD,SCHERALD,QAHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** internal doc — no external online sources. The subcommand surface + `version --json` were derived by running the built `iherald` binary (`iherald --help`, `iherald version --json`) on 2026-05-30; the 501-stub status of `POST /v1/webhooks/page` was confirmed by reading `iherald/internal/http/routes.go` (route registered with `HRD: "HRD-024"`) and the package comments in `iherald/cmd/iherald/main.go` + `iherald/internal/bindings/bindings.go`, plus `commons/branding.go` for the flavor identity. No routes or flags were invented; the not-yet-implemented status is stated honestly. Re-verify when HRD-024 lands the live paging handler.
