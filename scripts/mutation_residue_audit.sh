#!/usr/bin/env bash
# scripts/mutation_residue_audit.sh — Pre-push mutation-residue scanner.
#
# MANDATE: Helix Universal Constitution §11.4.84 + Herald §107.y
# (Working-Tree Quiescence Rule). Canonical Herald authority:
# docs/guides/HERALD_CONSTITUTION.md §107.y + CLAUDE.md §107.y.
#
# FORENSIC ANCHOR — the incident this scanner prevents:
#   On 2026-05-21 a logo-fix subagent (commit 72e81ab) ran in the shared
#   checkout while a paired §1.1 mutation gate had temporarily injected an
#   `// always pass` JWT-bypass mutation into commons_auth/middleware.go.
#   The subagent's `git add` swept the mutation residue into its commit and
#   it was pushed to all four mirrors before another agent caught it. The
#   SECURITY FIX (commit d5bd360) restored the verify path within the hour,
#   but the production-equivalent-binary-with-bypassed-JWT window is a real
#   security-defect window — small, non-zero, demonstrably exploitable.
#   A second occurrence-class was the 2026-05-27 concurrent-mutation incident.
#   This scanner is the constitutional outcome: a mutation marker that lands
#   in any tagged/pushed Herald commit is a CRITICAL defect regardless of how
#   briefly it persists.
#
# WHAT IT DOES (read-only, side-effect-free, idempotent):
#   Scans Herald source for the canonical §107.y mutation markers. If ANY
#   marker is found, it reports each hit as `file:line: <marker>` and exits
#   non-zero so the caller (push flow / pre-push hook) BLOCKS the push.
#   The scan also refuses (exit non-zero) if a mutation gate is mid-flight
#   (.git/MUTATION_IN_PROGRESS lockfile present) or if orphaned mutation
#   backup files (.w*meta-backup) or _mutated_ filenames exist.
#
# USAGE:
#   scripts/mutation_residue_audit.sh
#       Scan the current working tree + staged/index content (default).
#   scripts/mutation_residue_audit.sh <rev-a> <rev-b>
#       Additionally scan every commit in the range <rev-a>..<rev-b> for
#       markers introduced by those commits (pre-push range check).
#
# EXIT: 0 = clean (no residue). non-zero = residue found → push BLOCKED.
#
# This script NEVER mutates anything. It only reads (grep / git show / find).

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

hits=0

report() {
    # report <file> <line> <marker-description>
    echo "RESIDUE  $1:$2: $3"
    hits=$((hits + 1))
}

# ----------------------------------------------------------------------
# Scope: tracked source files, EXCLUDING the governance test files that
# legitimately contain the marker strings as documentation/anchors
# (the mutation gates themselves + this scanner + the constitution docs
# that quote the marker list). Those files describe the markers; they do
# not carry live residue. Everything else is in-scope.
# ----------------------------------------------------------------------
is_exempt() {
    # Exempt files that legitimately CONTAIN the marker strings as
    # documentation/anchors/plan-text rather than live residue. Live
    # residue can only sit in executable source (*.go, *.sh, *.py, *.sql,
    # etc.) — never in prose docs or their exported HTML/PDF/DOCX siblings.
    case "$1" in
        # This scanner + the mutation gates that emit the markers.
        scripts/mutation_residue_audit.sh) return 0 ;;
        tests/test_wave*_mutation_meta.sh) return 0 ;;
        tests/test_i8_usability_meta.sh) return 0 ;;
        # Constitution / governance prose (source + exported siblings).
        CLAUDE.md|AGENTS.md|QWEN.md) return 0 ;;
        docs/guides/HERALD_CONSTITUTION.md) return 0 ;;
        docs/research/*) return 0 ;;
        docs/CONTINUATION.md|docs/Issues.md|docs/Fixed.md|docs/Status.md) return 0 ;;
        docs/*_Summary.md) return 0 ;;
        # Spec + planning prose (the spec quotes the marker list verbatim;
        # the superpowers plans contain the literal mutation one-liners).
        docs/specs/*) return 0 ;;
        docs/superpowers/*) return 0 ;;
        # Exported documentation siblings (pandoc HTML/PDF/DOCX of any .md).
        *.html|*.pdf|*.docx) return 0 ;;
        *) return 1 ;;
    esac
}

# ----------------------------------------------------------------------
# Canonical §107.y mutation markers (literal grep patterns).
#   m_paired   — `MUTATED for paired` : the canonical paired-§1.1 marker
#   m_wave     — `MUTATED W<n>-M<n>`  : Wave 4b / Wave 6 / Wave 6.5 markers
#   m_genM     — `MUTATED M<n>`       : Wave 3 / Wave 4a generic markers
#   m_alwayspass — `// always pass`   : the literal JWT-bypass residue shape
#   m_mutation_go / m_mutation_sh — `// MUTATION` / `# MUTATION` annotations
# json.Marshal shortcut residue is handled separately (scoped + specific).
# ----------------------------------------------------------------------
PAT_MARKERS='MUTATED for paired|MUTATED W[0-9]+(\.[0-9]+)?-M[0-9]+|MUTATED W[0-9]|MUTATED M[0-9]|// always pass|// MUTATION|# MUTATION'

scan_tracked_markers() {
    # git grep -n --untracked over the WORKING TREE (tracked + untracked,
    # .gitignore-respecting so submodules/ + containers/ + build artefacts
    # are skipped) — §107.y mandates grepping the agent's OWN working tree,
    # which includes untracked-but-not-ignored files that may carry residue.
    # Exempt governance/prose files are filtered out.
    local line file lineno text
    while IFS= read -r line; do
        [ -z "${line}" ] && continue
        file="${line%%:*}"
        is_exempt "${file}" && continue
        lineno="$(printf '%s' "${line}" | cut -d: -f2)"
        text="$(printf '%s' "${line}" | cut -d: -f3-)"
        report "${file}" "${lineno}" "mutation marker: ${text}"
    done < <(git grep -n --untracked -E "${PAT_MARKERS}" -- . 2>/dev/null || true)
}

# ----------------------------------------------------------------------
# json.Marshal shortcut residue — Wave 4b TOON mutation (M2). Be precise:
# the gate injects the EXACT comment `MUTATED W4B-M2` next to a swapped
# `json.Marshal(v)` in the TOON branch. We grep for the specific injected
# comment within commons/ or commons_messaging/, NOT all json.Marshal
# (which is legitimate everywhere). The generic-marker scan above already
# catches `MUTATED W4B-M2`; this is a belt-and-suspenders scoped probe that
# also flags the documented bluff phrasing "delegate to json.Marshal".
# ----------------------------------------------------------------------
scan_toon_shortcut() {
    local line file lineno text
    while IFS= read -r line; do
        [ -z "${line}" ] && continue
        file="${line%%:*}"
        is_exempt "${file}" && continue
        lineno="$(printf '%s' "${line}" | cut -d: -f2)"
        text="$(printf '%s' "${line}" | cut -d: -f3-)"
        report "${file}" "${lineno}" "TOON json.Marshal shortcut residue: ${text}"
    done < <(git grep -n --untracked -E 'silently delegate to json\.Marshal|PASS-bluff recurrence' \
                 -- 'commons/*' 'commons_messaging/*' 2>/dev/null || true)
}

# ----------------------------------------------------------------------
# Orphaned mutation backup files (.w2meta-backup .w3meta-backup
# .w4meta-backup .w4bmeta-backup … any .w*meta-backup) — these are produced
# by the cp-backup mutation gates and MUST be removed on restore. A
# surviving backup file means a gate was interrupted mid-cycle.
# ----------------------------------------------------------------------
scan_backup_files() {
    local f
    while IFS= read -r f; do
        [ -z "${f}" ] && continue
        report "${f}" "0" "orphaned mutation backup file (interrupted gate)"
    done < <(find . -type f -name '*.w*meta-backup' \
                 -not -path './.git/*' -not -path './submodules/*' \
                 -not -path './containers/*' 2>/dev/null || true)
}

# ----------------------------------------------------------------------
# _mutated_ filename suffixes — tracked or untracked.
# ----------------------------------------------------------------------
scan_mutated_filenames() {
    local f
    while IFS= read -r f; do
        [ -z "${f}" ] && continue
        report "${f}" "0" "_mutated_ filename suffix"
    done < <(find . -type f -name '*_mutated_*' \
                 -not -path './.git/*' -not -path './submodules/*' \
                 -not -path './containers/*' 2>/dev/null || true)
}

# ----------------------------------------------------------------------
# Lockfile — a mutation gate is mid-flight. Pushing now risks committing
# the in-flight mutation residue. BLOCK.
# ----------------------------------------------------------------------
scan_lockfile() {
    if [ -f .git/MUTATION_IN_PROGRESS ]; then
        report ".git/MUTATION_IN_PROGRESS" "0" "mutation gate IN FLIGHT (lockfile present) — push BLOCKED until gate completes"
    fi
}

# ----------------------------------------------------------------------
# Optional commit-range scan: for `git diff <a>..<b>` ADDED lines that
# introduce a marker. Catches residue that landed in a commit even if the
# working tree was later cleaned (the working-tree scans above would miss
# an already-committed-and-cleaned marker that is still in the push range).
# ----------------------------------------------------------------------
scan_commit_range() {
    local a="$1" b="$2"
    local line cur_file
    cur_file="(unknown)"
    while IFS= read -r line; do
        case "${line}" in
            "+++ b/"*)
                cur_file="${line#+++ b/}"
                ;;
            "+"*)
                # Added line. Skip the +++ header (handled above).
                case "${line}" in "+++"*) continue ;; esac
                is_exempt "${cur_file}" && continue
                if printf '%s' "${line}" | grep -qE "${PAT_MARKERS}"; then
                    report "${cur_file}" "(added-in-range ${a}..${b})" "mutation marker in pushed commit: ${line#+}"
                fi
                ;;
        esac
    done < <(git diff "${a}..${b}" -- . 2>/dev/null || true)
}

# ----------------------------------------------------------------------
# Run all scans.
# ----------------------------------------------------------------------
echo "== mutation-residue audit (§11.4.84 / §107.y) =="
scan_lockfile
scan_tracked_markers
scan_toon_shortcut
scan_backup_files
scan_mutated_filenames

if [ "$#" -ge 2 ]; then
    echo "   (also scanning commit range $1..$2)"
    scan_commit_range "$1" "$2"
fi

echo "----"
if [ "${hits}" -eq 0 ]; then
    echo "PASS: no mutation residue — working tree quiescent (§107.y)"
    exit 0
else
    echo "FAIL: ${hits} mutation-residue hit(s) found — push BLOCKED per §11.4.84 / §107.y"
    echo "      Revert/amend the offending commit or restore the mutated file(s)"
    echo "      before pushing. See docs/guides/HERALD_CONSTITUTION.md §107.y."
    exit 1
fi
