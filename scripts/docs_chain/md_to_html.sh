#!/usr/bin/env bash
# docs_chain exec transform: markdown -> standalone HTML, replicating scripts/export_docs.sh's
# EXACT pandoc flags (logo print.css preserved) + SOURCE_DATE_EPOCH for reproducibility.
# docs_chain runs this with cwd=<project-root>, argv: <in_md_tmp> <out_html_tmp> <real_doc_dir>.
# IMPORTANT: in/out are STAGED TEMPS (a temp dir during `verify`), so the wrapper must NOT
# derive the doc's location from them — the real doc dir is passed explicitly as $3 so CSS/<img>
# resolution is IDENTICAL during sync and verify (→ deterministic, verify-stable output).
set -euo pipefail
in="$1"; out="$2"; realdir="${3:?real doc dir arg required}"
root="$PWD"
in_abs="$(cd "$(dirname "$in")" && pwd)/$(basename "$in")"
out_abs="$(cd "$(dirname "$out")" && pwd)/$(basename "$out")"
rel_css="$(python3 -c "import os.path; print(os.path.relpath('$root/assets/logo/print.css', '$root/$realdir'))")"
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-946684800}"
( cd "$root/$realdir" && pandoc --standalone --css="$rel_css" --metadata title="Herald" \
    -f gfm -t html5 -o "$out_abs" "$in_abs" )
