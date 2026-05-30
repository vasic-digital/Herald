<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Herald — `qaherald` Flavor Guide (Operator)

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-30 |
| Last modified | 2026-05-30 |
| Status | active |
| Status summary | Nano-detail operator reference for `qaherald` (QA Herald) — a CLI-only flavor (DefaultPort=0) that is Herald's autonomous QA bot. Documents `run` (scenario harness → docs/qa evidence), `mtproto` (the MTProto user-client session lifecycle: `login`/`whoami`/`logout`) and WHY MTProto is required (Telegram's bot-privacy boundary), `lifecycle` (the 15-scenario lifecycle driver — currently a SKELETON), and `version`. ANTI-BLUFF: derived from the built `qaherald` binary (`qaherald --help`, per-subcommand `--help`, `version --json`) + `commons/branding.go`; the lifecycle SKELETON status is quoted verbatim from the binary's own help. |
| Issues | (none specific to this guide) |
| Continuation | Bump when `lifecycle` T2 (messenger) + T3 (scenarios) wire the real driver and when live MTProto evidence lands under `docs/qa/`. |

## Table of contents

- [§1. What `qaherald` is](#1-what-qaherald-is)
- [§2. The subcommand surface](#2-the-subcommand-surface)
- [§3. `version`](#3-version)
- [§4. Why MTProto (and not the Bot API)](#4-why-mtproto-and-not-the-bot-api)
- [§5. `mtproto` — the user-client session lifecycle](#5-mtproto--the-user-client-session-lifecycle)
- [§6. `run` — the scenario harness](#6-run--the-scenario-harness)
- [§7. `lifecycle` — the 15-scenario driver (skeleton)](#7-lifecycle--the-15-scenario-driver-skeleton)
- [§8. References](#8-references)

---

## §1. What `qaherald` is

`qaherald` is **QA Herald** — flavor key `qa`, prefix `QHR`, **CLI-only** (DefaultPort=0 — it drives external services, it does not serve HTTP itself). Per `commons/branding.go` its mission is "QA bot — pherald ↔ Telegram round-trip automation + docs/qa/ evidence". It is the binary that satisfies Herald's §107.x evidence mandate: it drives `pherald` ↔ Telegram round-trips end-to-end, records bidirectional transcripts + content-addressed attachments under `docs/qa/<run-id>/`, and emits a Markdown report. Per HelixConstitution §11.4.98 (Full-Automation Anti-Bluff Mandate), it must do this **fully autonomously** — which is exactly why it speaks MTProto.

Build it:

```bash
go build -o /tmp/qaherald ./qaherald/cmd/qaherald
```

## §2. The subcommand surface

Verbatim from `qaherald --help`:

| Subcommand | What it does | State |
|---|---|---|
| `run` | Run qaherald scenarios against the configured `pherald` + Telegram bot | live |
| `mtproto` | Manage the MTProto user-client session used by qaherald (`login`/`whoami`/`logout`) | `login` interactive; `whoami`/`logout` autonomous |
| `lifecycle` | Run the 15-scenario lifecycle test against `pherald listen` via a 2nd Telegram bot | **SKELETON — driver not wired (T2/T3)** |
| `version` | Print QA Herald version + build info | live |
| `completion` | Generate shell autocompletion (Cobra built-in) | live |

## §3. `version`

```bash
$ qaherald version --json
{"arch":"arm64","binary":"qaherald","build_time":"unknown","commit":"unknown","flavor":"qa","go_version":"go1.26.2","os":"darwin","version":"0.0.0-dev"}
```

## §4. Why MTProto (and not the Bot API)

Quoted from `qaherald mtproto --help`: Telegram's bot privacy boundary blocks a bot from observing another bot's messages in non-DM contexts. The QA flavor MUST drive the **system** bot (the one `pherald listen` runs) end-to-end; the only autonomous way to do that is via a **real user account** speaking MTProto. A second bot cannot see the first bot's group messages, so a bot-to-bot QA loop is a dead end — hence `qaherald` carries an MTProto user-client.

This is consistent with §11.4.98: a test that needs a human to type a Telegram message is a PASS-bluff at the automation layer. The MTProto user-account is the autonomous driver that removes the human from the loop.

## §5. `mtproto` — the user-client session lifecycle

`qaherald mtproto` manages the persisted MTProto user-client session. Lifecycle:

1. **One-time bootstrap (interactive):** `qaherald mtproto login` — the operator enters Telegram's SMS / app-push code. This is the single permitted manual step (credential bootstrap, not test-driving — §11.4.98's allowed exception).
2. **Health check (autonomous):** `qaherald mtproto whoami` — prints the connected user identity.
3. **Teardown / rotate (autonomous):** `qaherald mtproto logout` — server-side `LogOut` + removes the local session file.

**Required environment (all four):**

| Variable | Source / meaning |
|---|---|
| `HERALD_MTPROTO_APP_ID` | From https://my.telegram.org/apps (integer) |
| `HERALD_MTPROTO_APP_HASH` | From https://my.telegram.org/apps (32-char hex) |
| `HERALD_MTPROTO_PHONE` | E.164 phone of the QA user account |
| `HERALD_MTPROTO_PASSWORD` | Cloud 2FA password (empty if 2FA off) |

The session file lives at `~/.config/herald/mtproto.session` (mode 0600). It is never committed, never echoed, and protected at rest only by filesystem ACL.

```bash
export HERALD_MTPROTO_APP_ID=...
export HERALD_MTPROTO_APP_HASH=...
export HERALD_MTPROTO_PHONE=+15551234567
qaherald mtproto login     # interactive — enter the Telegram code once
qaherald mtproto whoami    # autonomous health check
```

> SAFETY: the MTProto path uses a real Telegram account. Follow the credential-setup guidance in `docs/guides/OPERATOR_CREDENTIALS.md` (and the Telegram-official cross-references there) — using a VoIP number or skipping the recovery-email setup risks an account ban.

## §6. `run` — the scenario harness

`qaherald run` drives the configured set of scenarios end-to-end: constructs CloudEvents, POSTs them via HTTPS+JWT (with TOON content negotiation per Wave 4b), observes Telegram-side delivery, records a bidirectional transcript + content-addressed attachments under `docs/qa/<run-id>/`, and emits a Markdown report. Per §107.x it is the canonical producer of the `docs/qa/<run-id>/` evidence artefact.

**Exit code is the contract:** 0 when every scenario PASSes, non-zero on any FAIL — a silent FAIL would be a §11.4 PASS-bluff at the harness layer.

| Flag | Env | Meaning |
|---|---|---|
| `--herald-url string` | `HERALD_BASE_URL` | pherald base URL (default `https://localhost:7443`) |
| `--herald-token string` | `HERALD_JWT_SECRET` | JWT HMAC secret for pherald — **REQUIRED** |
| `--tg-token string` | `HERALD_TGRAM_BOT_TOKEN` | Telegram bot token — **REQUIRED** |
| `--tg-chat int` | `HERALD_TGRAM_CHAT_ID` | Telegram chat ID — **REQUIRED** |
| `--scenario string` | — | Scenario name (e.g. `happy-path-single-channel`) or `all` (default `all`) |
| `--scenario-timeout duration` | — | Per-scenario timeout (default `1m`) |
| `--out-dir string` | — | Parent directory for the `<run-id>/` subdirectory (default `docs/qa`) |

```bash
qaherald run --scenario all \
    --herald-url https://localhost:7443 \
    --herald-token "$HERALD_JWT_SECRET" \
    --tg-token "$HERALD_TGRAM_BOT_TOKEN" --tg-chat 987654321
```

## §7. `lifecycle` — the 15-scenario driver (skeleton)

`qaherald lifecycle` automates the S01..S15 scenarios from `tests/test_wave6.5_lifecycle.sh` by posting each input via a 2nd Telegram bot (`HERALD_QA_BOT_TOKEN`) and asserting `pherald`'s reply + filesystem mutation. Outputs land under `docs/qa/HRD-101-lifecycle-<run-id>/` (`transcript.jsonl`, `report.md`, content-addressed `attachments/`).

**State: SKELETON only.** Quoted verbatim from the binary's help: "T1 status: SKELETON only — T2 (messenger) and T3 (scenarios) wire the actual driver. Running this command today resolves env fallbacks, validates required flags, creates the OutDir, and exits with a 'not-wired-yet' message." Do not rely on `lifecycle` for real QA evidence yet — use `run` (§6).

Pre-reqs documented by the help (for when T2/T3 land): `pherald listen` running with `--qa-out-dir` set; the qa-bot in the same group as the pherald-bot; the qa-bot's Privacy Mode DISABLED (via `@BotFather → /setprivacy → Disable`); `HERALD_OPERATOR_IDS` containing the qa-bot's user-id. Notable flags include `--qa-bot-token` (env `HERALD_QA_BOT_TOKEN`), `--chat-id`, `--scenarios` (subset of S01..S15), `--run-id`, and `--skip-preflight` (DEV ONLY — forbidden in prod). Run `qaherald lifecycle --help` for the full set.

## §8. References

- Source: `qaherald/cmd/qaherald/main.go`, `mtproto_cmd.go`, `run` + `lifecycle` command files.
- Branding: `commons/branding.go` (flavor `qa`, DefaultPort=0).
- Evidence mandate: Herald §107.x (docs/qa evidence) + HelixConstitution §11.4.98 (full-automation anti-bluff).
- MTProto credentials: `docs/guides/OPERATOR_CREDENTIALS.md` (MTProto bootstrap + Telegram-official safety cross-references); https://my.telegram.org/apps (app id/hash source).
- Companion flavor guides: `docs/guides/{PHERALD,SHERALD,CHERALD,BHERALD,RHERALD,IHERALD,SCHERALD}.md`.

## Sources verified

**Verified 2026-05-30:** Telegram developer portal — https://my.telegram.org/apps (the source of `HERALD_MTPROTO_APP_ID` / `HERALD_MTPROTO_APP_HASH`, cross-referenced because qaherald is a risk-classified Telegram-touching tool). All subcommand, flag, default, env-var, and state claims (`lifecycle` SKELETON, `mtproto login` interactive vs `whoami`/`logout` autonomous, the MTProto-vs-Bot-API rationale) were derived by running the built `qaherald` binary (`qaherald --help`; `qaherald run|mtproto|mtproto login|lifecycle --help`; `qaherald version --json`) and reading `commons/branding.go` on 2026-05-30 — the SKELETON status is quoted verbatim from the binary's own help. No flags were invented. **Re-verification cadence (per §11.4.99 (C)):** Telegram is a risk-classified service → **90-day staleness** (next due **2026-08-28**) for the my.telegram.org app-id/hash + account-safety guidance; the rest tracks the `qaherald` Cobra surface — re-verify on any subcommand change (notably when `lifecycle` T2/T3 land).
