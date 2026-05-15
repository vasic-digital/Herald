# Herald

Ingesting system events and reliably fanning them out to multiple notification channels so every alert reaches the right destination without confusion.

---

## Status

Herald is **pre-implementation** (2026-05-15). The repository currently contains:

- This README.
- An MVP specification stub at [`docs/specs/mvp/specification.md`](docs/specs/mvp/specification.md) — substantive sections are still TBD.
- Project-specific Constitution + operator/agent guides under [`docs/guides/`](docs/guides/).
- Mirror declarations at [`upstreams/`](upstreams/) — one shell script per host that exports `UPSTREAMABLE_REPOSITORY`.
- The inheritance gate + paired mutation meta-test at [`tests/`](tests/).
- `.gitignore` tuned for Go (`*.test`, `go.work*`, `coverage.*`) and `.DS_Store`.

There is no `go.mod`, no Go source code, and no build tooling yet. Intended language is Go; standard `cmd/` + `internal/` layout will be used when scaffolding starts.

## Mission

Herald sits between systems that **emit events** and humans/services that **want to be told about them**. It guarantees:

- Every event is delivered to every configured channel for its category.
- Duplicate suppression and routing are explicit, not accidental.
- Delivery failures on one channel never block delivery to other channels.

## Deployment model

Herald is designed to be consumed as a **git submodule** of a larger project (the "consuming project"). The consuming project provides:

- The **Helix Constitution** submodule at its own `<consuming-project>/constitution/`.
- Its own CI / build orchestration that drives Herald.
- Any project-wide configuration (credentials, deployment targets, channel registries).

Herald therefore **does not carry its own copy** of the constitution. See [`docs/guides/CONSTITUTION_INHERITANCE.md`](docs/guides/CONSTITUTION_INHERITANCE.md) for the full rationale and the discovery contract that lets Herald locate the constitution at runtime from any nested depth.

## Governance — Helix Constitution inheritance

Herald inherits unconditionally from the [Helix Universal Constitution](https://github.com/HelixDevelopment/HelixConstitution). Inheritance is enforced by a paired gate + mutation meta-test under `tests/` (see below). Herald-specific extensions live in [`docs/guides/HERALD_CONSTITUTION.md`](docs/guides/HERALD_CONSTITUTION.md); there are currently no overrides of any universal clause.

Key invariants Herald inherits from the constitution:

- **No bluffing** — every PASS carries positive evidence (§11.4).
- **Mutation-paired gates** — every new gate has a paired mutation proving it catches regressions (§1.1).
- **No guessing language** — `likely`, `probably`, `maybe`, `seems`, `appears` are forbidden when reporting causes (§11.4.6).
- **Credentials never tracked** — `.env` git-ignored, runtime-load only (§11.4.10).
- **Multi-upstream push** — every commit fans out to GitHub + GitLab + GitFlic + GitVerse (§2.1).
- **Hardlinked backup before destructive ops** (§9).

The constitution lives at <https://github.com/HelixDevelopment/HelixConstitution> and is mirrored to GitLab, GitFlic, and GitVerse.

## Repository layout

```
Herald/
├── README.md                                  # this file
├── CLAUDE.md                                  # guidance for Claude Code agents
├── AGENTS.md                                  # guidance for generic CLI agents
├── LICENSE
├── .gitignore
├── docs/
│   ├── guides/
│   │   ├── HERALD_CONSTITUTION.md             # Herald's project constitution (extends Helix)
│   │   └── CONSTITUTION_INHERITANCE.md        # operator/agent guide for the inheritance contract
│   └── specs/
│       └── mvp/
│           └── specification.md               # MVP spec (TBD)
├── upstreams/                                 # Herald's mirror declarations
│   ├── GitHub.sh
│   ├── GitLab.sh
│   ├── GitFlic.sh
│   └── GitVerse.sh
└── tests/
    ├── test_constitution_inheritance.sh       # inheritance gate
    └── test_constitution_inheritance_meta.sh  # paired mutation meta-test (§1.1)
```

## Quickstart for developers and agents

### 1. Place a copy of the Helix Constitution alongside Herald

If you cloned Herald standalone (not as a submodule of a larger project), put a clone of the constitution **next to** Herald (not inside it — see [§104 of the Herald Constitution](docs/guides/HERALD_CONSTITUTION.md#§104-no-embedded-constitution-extends-universal-§3)):

```bash
git clone git@github.com:HelixDevelopment/HelixConstitution.git \
    $(dirname "$PWD")/constitution
```

This is only needed for standalone work. When Herald is consumed as a submodule of a larger project, that project already provides `<parent>/constitution/`.

### 2. Verify the inheritance contract

```bash
bash tests/test_constitution_inheritance.sh        # gate (7 invariants)
bash tests/test_constitution_inheritance_meta.sh   # paired §1.1 mutation proof
```

Both MUST exit 0. The gate prints `PASS  …` / `FAIL  …` per invariant and a summary line. The meta-test prints `✓ META-TEST PASS` when the gate correctly fails on a mutated constitution.

If `I1` fails (constitution not found), follow the message — clone the constitution alongside Herald.

### 3. Read the inherited rules

In this order, read fully before submitting any change:

1. `<discovered-constitution>/CLAUDE.md` + `Constitution.md` — universal Helix rules.
2. `<discovered-constitution>/AGENTS.md` — anti-bluff, no-guessing, paired mutations.
3. This README — Herald overview.
4. [`CLAUDE.md`](CLAUDE.md) / [`AGENTS.md`](AGENTS.md) — Herald-specific guidance.
5. [`docs/guides/HERALD_CONSTITUTION.md`](docs/guides/HERALD_CONSTITUTION.md) — Herald's articles §101–§105.
6. [`docs/guides/CONSTITUTION_INHERITANCE.md`](docs/guides/CONSTITUTION_INHERITANCE.md) — the discovery contract and gate semantics.
7. [`docs/specs/mvp/specification.md`](docs/specs/mvp/specification.md) — MVP spec (TBD).

## Mirror & push convention

Herald is mirrored to four hosts. The `origin` remote is **fan-out**: one fetch URL + four push URLs. A single `git push origin main` propagates to every mirror in one operation.

| Remote name | URL |
|---|---|
| `github` | `git@github.com:vasic-digital/Herald.git` |
| `gitlab` | `git@gitlab.com:vasic-digital/herald.git` |
| `gitflic` | `git@gitflic.ru:vasic-digital/herald.git` |
| `gitverse` | `git@gitverse.ru:vasic-digital/Herald.git` |
| `origin` | (fetch from `github`; push fans out to all four) |

Each entry under `upstreams/` is a shell script that exports a single `UPSTREAMABLE_REPOSITORY=…` URL and is meant to be **sourced**, not executed for its output. Capitalization matches each host's brand (GitFlic, GitVerse); do not normalize to lowercase or collapse into one file — external mirror-push tooling keys on the per-file split.

If you ever need to rebuild the fan-out configuration, the constitution submodule ships `install_upstreams.sh` that consumes `Upstreams/*.sh` declarations and configures git remotes accordingly; the same pattern can be adapted for Herald.

## License

[`LICENSE`](LICENSE) — see file for terms.

## Contact / contribution

Substantive contributions land via PRs on GitHub; mirrors are read-only for external consumers. Inheritance rules and the gate apply to every PR.
