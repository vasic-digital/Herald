#!/usr/bin/env bash
# docs_chain exec transform: markdown -> DOCX (embeds images), replicating export_docs.sh.
# argv: <in_md_tmp> <out_docx_tmp> <real_doc_dir>.
set -euo pipefail
in="$1"; out="$2"; realdir="${3:?real doc dir arg required}"
root="$PWD"
in_abs="$(cd "$(dirname "$in")" && pwd)/$(basename "$in")"
out_abs="$(cd "$(dirname "$out")" && pwd)/$(basename "$out")"
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-946684800}"
( cd "$root/$realdir" && pandoc -f gfm -t docx --metadata title="Herald" -o "$out_abs" "$in_abs" )
