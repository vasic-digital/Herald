<div align="center">

<img src="../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# CodeGraph Integration

| Field | Value |
|---|---|
| Revision | 1 |
| Created | 2026-05-20 |
| Last modified | 2026-05-20 |
| Status | active |
| Status summary | Initial CodeGraph integration per user mandate 2026-05-20 + Universal §11.4.78 (CodeGraph code-intelligence mandate). Herald is indexed (48 Go files / 775 nodes / 951 edges). Setup + validate scripts shipped. CLI-agent wiring documented for Claude Code (primary), OpenCode, Kimi CLI, Crush, Qwen Code. Anti-bluff `scripts/codegraph_validate.sh` proves the index returns real Herald symbols with 7/7 PASS at landing time. |
| Issues | none |
| Issues summary | — |
| Fixed | initial integration |
| Fixed summary | — |
| Continuation | Re-run `scripts/codegraph_setup.sh` after any non-trivial code change to keep the graph fresh. The setup script auto-detects existing `.codegraph/` and runs `sync` rather than full re-index. |

## Table of contents

- [§1. What CodeGraph is](#1-what-codegraph-is)
- [§2. Setup](#2-setup)
- [§3. Anti-bluff validation](#3-anti-bluff-validation)
- [§4. CLI agent wiring](#4-cli-agent-wiring)
  - [4.1 Claude Code (primary)](#41-claude-code-primary)
  - [4.2 OpenCode](#42-opencode)
  - [4.3 Codex CLI](#43-codex-cli)
  - [4.4 Cursor](#44-cursor)
  - [4.5 Kimi CLI](#45-kimi-cli)
  - [4.6 Crush](#46-crush)
  - [4.7 Qwen Code](#47-qwen-code)
- [§5. Daily use](#5-daily-use)
- [§6. Forensic anchors + composition](#6-forensic-anchors--composition)

## §1. What CodeGraph is

CodeGraph (`@colbymchenry/codegraph`, MIT, `https://github.com/colbymchenry/codegraph`) is a local pre-indexed knowledge graph for source code. Per its README benchmark, it produces ≈ 92% fewer Claude Code tool calls and ≈ 71% faster Explore-agent runs against 6 reference codebases.

For Herald specifically: instead of having an Explore subagent grep + glob + Read 40 files to answer "how does the constitution-rule Evaluator dispatch work?", CodeGraph returns the exact symbol locations and edges in one query.

Per Universal Constitution §11.4.78 (CodeGraph code-intelligence mandate) every consuming project should adopt CodeGraph. This document is Herald's adoption record.

## §2. Setup

Prerequisites: Node.js 18+ on `PATH`.

```bash
cd <herald-root>
scripts/codegraph_setup.sh
```

The script is idempotent:

- If `.codegraph/` does not exist → runs `init` + full `index`.
- If `.codegraph/` exists → runs `sync` (incremental update of changed files).
- Always finishes with `status` to print the graph stats.

At landing time the index reports:

```
  Nodes:     775
  Edges:     951
  DB Size:   1.54 MB
  Files by Language: go (48)
```

The `.codegraph/codegraph.db` is gitignored (`.codegraph/.gitignore` ships its own ignore rules). `.codegraph/config.json` IS tracked — it carries the include/exclude globs.

## §3. Anti-bluff validation

Per Universal §11.4 + §11.4.69, a CodeGraph install that "looks installed" but returns empty results for real Herald symbols is the exact bluff §11.4 forbids. The validation script asserts the index is functioning:

```bash
scripts/codegraph_validate.sh
```

Probes 6 known Herald symbols + 1 negative control. PASSes only when:

- Every positive probe (`Evaluator`, `ConstitutionStore`, `Record`, `WithTenantContext`, `QuickstartBoot`, `Server`) returns ≥ 1 hit AND
- The negative control (a definitely-absent symbol) returns ≤ 1 hit.

At landing time: **7/7 PASS**.

Re-run after any non-trivial code change. A regression here means the index is stale — re-run `scripts/codegraph_setup.sh` to refresh.

## §4. CLI agent wiring

CodeGraph exposes itself to AI agents via the **Model Context Protocol** (MCP) server. The agent connects to `codegraph serve --mcp` and gains a `codegraph` tool that maps to the same query semantics as the CLI.

### 4.1 Claude Code (primary)

Add to `.claude/settings.json` (project-local) OR `~/.claude/settings.json` (user-global):

```json
{
  "mcpServers": {
    "codegraph": {
      "command": "npx",
      "args": ["-y", "@colbymchenry/codegraph", "serve", "--mcp"],
      "env": {}
    }
  }
}
```

Restart Claude Code. Verify with `/mcp` — `codegraph` should appear as a connected server. Explore agents will now have access to the `codegraph` tool.

### 4.2 OpenCode

OpenCode honors the same MCP configuration. Add to `~/.opencode/config.json`:

```json
{
  "mcpServers": {
    "codegraph": {
      "command": "npx",
      "args": ["-y", "@colbymchenry/codegraph", "serve", "--mcp"]
    }
  }
}
```

### 4.3 Codex CLI

Per the CodeGraph README, Codex CLI is supported out-of-the-box by the package's interactive installer:

```bash
npx -y @colbymchenry/codegraph install
```

Select Codex CLI when prompted.

### 4.4 Cursor

Cursor supports MCP via its settings UI: **Settings → MCP → Add Server**, then paste the same `command` + `args` as in §4.1.

### 4.5 Kimi CLI

Kimi CLI's MCP support is configured via `~/.kimi/mcp.json` (subject to upstream changes):

```json
{
  "servers": {
    "codegraph": {
      "command": "npx",
      "args": ["-y", "@colbymchenry/codegraph", "serve", "--mcp"]
    }
  }
}
```

If Kimi CLI's MCP path differs from the canonical Claude Code shape (the spec is still evolving), the operator MUST adjust per the active Kimi CLI version's docs. Filed as `HRD-083` if the wiring needs Herald-side code.

### 4.6 Crush

Crush's MCP wiring follows the same pattern. Consult Crush's MCP docs for the exact config-file path; the `command`/`args` payload above is the same.

### 4.7 Qwen Code

Qwen Code MCP support is documented in the parent constitution submodule's `QWEN.md`. Add the same MCP server block to `~/.qwen/mcp.json` (or per active Qwen Code version's MCP config path):

```json
{
  "mcpServers": {
    "codegraph": {
      "command": "npx",
      "args": ["-y", "@colbymchenry/codegraph", "serve", "--mcp"]
    }
  }
}
```

## §5. Daily use

- After significant changes: `scripts/codegraph_setup.sh` (runs `sync`).
- Quick stats: `scripts/codegraph_setup.sh status`.
- Anti-bluff re-validation: `scripts/codegraph_validate.sh`.
- Visualise the graph: `npx -y @colbymchenry/codegraph visualize` (opens browser).

If an Explore subagent reports stale results despite CodeGraph being present, run `sync` first. A `mark-dirty` + `sync-if-dirty` post-commit hook can automate this — track under a follow-up HRD if Herald needs it (`HRD-084`).

## §6. Forensic anchors + composition

- **Universal §11.4.78** — CodeGraph code-intelligence mandate (constitution submodule `Constitution.md`).
- **§11.4.74** — Submodule-catalogue-first discovery. CodeGraph is a third-party MIT-licensed Submodule-equivalent (`npm` package); the Catalogue-Check verdict for "code-intelligence pre-indexing" is `extend external standard` rather than `extend vasic-digital` because no `vasic-digital/*` repo provides this capability.
- **§11.4 / §11.4.69** — Anti-bluff covenant. `scripts/codegraph_validate.sh` is the captured-evidence proof per §11.4.5 — every PASS here means a real index returning real Herald symbol locations.
- **§11.4.65** — Universal Markdown export. This guide's HTML + PDF + DOCX siblings ship in the same commit.
- **§11.4.20** — Subagent-driven-by-default. CodeGraph is the substrate that makes the subagent default cheap (≈ 92% fewer tool calls per its benchmarks); the two clauses compose: subagents-by-default for parallelism + CodeGraph for per-subagent efficiency.
