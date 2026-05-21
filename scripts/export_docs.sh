#!/usr/bin/env bash
# Herald docs export — regenerate HTML/PDF/DOCX siblings for every tracked .md
# that already has a sibling .html/.pdf/.docx in git.
#
# Engines:
#   - HTML : pandoc (standalone, embedded CSS) + assets/logo/print.css
#   - PDF  : pandoc --pdf-engine=weasyprint (resolves relative <img> srcs)
#   - DOCX : pandoc (embeds images directly)
#
# Idempotent. Safe to run repeatedly.
#
# Usage:
#   scripts/export_docs.sh                # regenerate every tracked artefact
#   scripts/export_docs.sh path/to/file.md  # regenerate one doc's siblings
#   scripts/export_docs.sh --list         # dry-run (list what would regenerate)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

CSS="$REPO_ROOT/assets/logo/print.css"

if ! command -v pandoc >/dev/null 2>&1; then
    echo "ERROR: pandoc not found in PATH" >&2
    exit 2
fi

PDF_ENGINE=""
if command -v weasyprint >/dev/null 2>&1; then
    PDF_ENGINE="weasyprint"
elif command -v wkhtmltopdf >/dev/null 2>&1; then
    PDF_ENGINE="wkhtmltopdf"
fi

regen_one() {
    local md="$1"
    local base="${md%.md}"
    local dir
    dir="$(dirname "$md")"
    local stem
    stem="$(basename "$base")"

    # The Markdown contains <img src="..relative.."> — pandoc resolves these
    # relative to the .md file location, so we must cd into the .md's
    # directory when invoking pandoc so the output references work.
    local rel_md
    rel_md="$(basename "$md")"
    local rel_css
    rel_css="$(python3 -c "import os.path; print(os.path.relpath('$CSS', '$dir'))")"

    local emitted=()

    if [ -f "${base}.html" ] || [ "${REGEN_ALL:-0}" = "1" ]; then
        ( cd "$dir" && pandoc --standalone --css="$rel_css" \
            --metadata title="Herald" \
            -f gfm -t html5 \
            -o "${stem}.html" "$rel_md" )
        emitted+=("${base}.html")
    fi

    if [ -f "${base}.pdf" ] || [ "${REGEN_ALL:-0}" = "1" ]; then
        if [ -n "$PDF_ENGINE" ]; then
            ( cd "$dir" && pandoc --pdf-engine="$PDF_ENGINE" \
                --css="$rel_css" \
                --metadata title="Herald" \
                -f gfm \
                -o "${stem}.pdf" "$rel_md" ) || {
                    echo "  WARN: PDF regen failed for $md" >&2
                    return 0
                }
            emitted+=("${base}.pdf")
        else
            echo "  SKIP: no PDF engine (weasyprint/wkhtmltopdf) — $md.pdf left stale" >&2
        fi
    fi

    if [ -f "${base}.docx" ] || [ "${REGEN_ALL:-0}" = "1" ]; then
        ( cd "$dir" && pandoc -f gfm -t docx \
            --metadata title="Herald" \
            -o "${stem}.docx" "$rel_md" )
        emitted+=("${base}.docx")
    fi

    if [ "${#emitted[@]}" -gt 0 ]; then
        printf '  OK   %s\n' "$md"
        for e in "${emitted[@]}"; do printf '         -> %s\n' "$e"; done
    fi
}

list_one() {
    local md="$1"
    local base="${md%.md}"
    local artefacts=()
    for ext in html pdf docx; do
        [ -f "${base}.${ext}" ] && artefacts+=("${base}.${ext}")
    done
    if [ "${#artefacts[@]}" -gt 0 ]; then
        printf '%s -> %s\n' "$md" "${artefacts[*]}"
    fi
}

if [ "${1:-}" = "--list" ]; then
    git ls-files '*.md' | while read -r md; do list_one "$md"; done
    exit 0
fi

# Exclusion filter mirroring branding_inject_logo.py.
should_process() {
    local md="$1"
    case "$md" in
        submodules/*|containers/*|constitutable/*|docs/diary/*) return 1 ;;
        LICENSE|LICENSE.md) return 1 ;;
    esac
    return 0
}

if [ $# -gt 0 ]; then
    for md in "$@"; do
        should_process "$md" || { echo "  SKIP $md (excluded)"; continue; }
        [ -f "$md" ] || { echo "  MISS $md"; continue; }
        regen_one "$md"
    done
else
    git ls-files '*.md' | while read -r md; do
        should_process "$md" || continue
        # Only regenerate if at least one sibling exists.
        base="${md%.md}"
        if [ -f "${base}.html" ] || [ -f "${base}.pdf" ] || [ -f "${base}.docx" ]; then
            regen_one "$md"
        fi
    done
fi
