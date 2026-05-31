#!/usr/bin/env bash
# docs_chain exec transform: markdown -> PDF (weasyprint), replicating export_docs.sh.
# argv: <in_md_tmp> <out_pdf_tmp> <real_doc_dir>. pandoc infers PDF from a .pdf extension,
# which the staged temp lacks — produce to a .pdf temp then mv to the extensionless target.
set -euo pipefail
in="$1"; out="$2"; realdir="${3:?real doc dir arg required}"
root="$PWD"
in_abs="$(cd "$(dirname "$in")" && pwd)/$(basename "$in")"
out_abs="$(cd "$(dirname "$out")" && pwd)/$(basename "$out")"
rel_css="$(python3 -c "import os.path; print(os.path.relpath('$root/assets/logo/print.css', '$root/$realdir'))")"
export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-946684800}"
( cd "$root/$realdir" && pandoc --pdf-engine=weasyprint --css="$rel_css" --metadata title="Herald" \
    -f gfm -o "${out_abs}.pdf" "$in_abs" && mv -f "${out_abs}.pdf" "$out_abs" )
