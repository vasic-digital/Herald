#!/usr/bin/env python3
"""
Herald branding — idempotently inject the centered logo header into each
tracked Markdown document.

- Skips files that already contain `herald_logo_square` (idempotent).
- Skips submodules/, containers/, constitutable/, docs/diary/, LICENSE.
- Honours YAML front-matter: inserts AFTER the closing `---`.
- Computes the relative path to assets/logo/herald_logo_square_128.png based
  on the doc's depth.
"""
import os
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
LOGO_REL = "assets/logo/herald_logo_square_128.png"
LOGO_MARKER = "herald_logo_square"

# Legacy header (raw <p align="center"><img>) — pandoc dropped it for docx.
# Detected here so we can upgrade to the canonical format below.
LEGACY_HEADER_RE = re.compile(
    r'^<p align="center"><img src="[^"]*herald_logo_square_128\.png"[^>]*/></p>\n\n',
    re.MULTILINE,
)


def rel_to_logo(md_path: Path) -> str:
    """Return relative path from md_path's directory to LOGO_REL."""
    md_dir = md_path.parent
    logo_abs = REPO_ROOT / LOGO_REL
    rel = os.path.relpath(logo_abs, start=md_dir)
    # Normalize to forward slashes (markdown is portable).
    return rel.replace(os.sep, "/")


def header_block(rel_path: str) -> str:
    """Pandoc-friendly logo header.

    Format:
      <div align="center">

      ![Herald](RELPATH){width=96px height=96px}

      </div>

    Why this shape:
      - <div align="center"> renders centered in raw HTML (GitHub, browsers).
      - Pandoc's gfm + markdown readers treat block-level HTML around a blank
        line as a wrapper and parse the inner markdown image — so the image
        is properly embedded into HTML, PDF (weasyprint), and DOCX.
      - {width=96px height=96px} is pandoc's attribute syntax; passed through
        to every output format.
    """
    return (
        f'<div align="center">\n'
        f'\n'
        f'![Herald]({rel_path}){{width=96px height=96px}}\n'
        f'\n'
        f'</div>\n'
        f'\n'
    )


def already_has_logo(content: str) -> bool:
    """Return True only if the CANONICAL header is present.

    A legacy raw-<p> header is treated as "needs upgrade" — not yet present.
    """
    head = content[:2000]
    if LEGACY_HEADER_RE.match(content):
        return False
    return LOGO_MARKER in head and "![Herald](" in head


def insert_after_front_matter(content: str, block: str) -> str:
    """If the doc starts with YAML front-matter (--- ... ---), insert AFTER it.
    Otherwise insert at the very top.
    """
    if content.startswith("---\n"):
        # find the closing --- on its own line (line 2+).
        lines = content.split("\n")
        for i in range(1, min(len(lines), 200)):
            if lines[i].strip() == "---":
                # insert after line i (the closing fence).
                head = "\n".join(lines[: i + 1]) + "\n"
                tail = "\n".join(lines[i + 1 :])
                # ensure exactly one blank line between fence and block.
                if tail.startswith("\n"):
                    return head + "\n" + block + tail.lstrip("\n")
                return head + "\n" + block + tail
    return block + content


def process(md_path: Path) -> str:
    content = md_path.read_text(encoding="utf-8")
    rel = rel_to_logo(md_path)
    block = header_block(rel)

    # Upgrade legacy raw-<p> header in place.
    if LEGACY_HEADER_RE.match(content):
        new_content = LEGACY_HEADER_RE.sub(block, content, count=1)
        md_path.write_text(new_content, encoding="utf-8")
        return f"upgraded legacy header ({rel})"

    if already_has_logo(content):
        return "skipped (already has canonical logo)"

    new_content = insert_after_front_matter(content, block)
    md_path.write_text(new_content, encoding="utf-8")
    return f"injected ({rel})"


# Exclusion rules: paths that match any of these are skipped.
EXCLUDED_PREFIXES = (
    "submodules/",
    "containers/",
    "constitutable/",
    "docs/diary/",
)
EXCLUDED_NAMES = {"LICENSE", "LICENSE.md"}


def is_excluded(rel_md: str) -> bool:
    if any(rel_md.startswith(p) for p in EXCLUDED_PREFIXES):
        return True
    if Path(rel_md).name in EXCLUDED_NAMES:
        return True
    return False


def main(argv):
    if len(argv) < 2:
        print("usage: branding_inject_logo.py <md_path> [<md_path> ...]", file=sys.stderr)
        sys.exit(2)
    for arg in argv[1:]:
        rel = arg
        if is_excluded(rel):
            print(f"  SKIP  {rel}  (excluded)")
            continue
        p = REPO_ROOT / rel
        if not p.exists():
            print(f"  MISS  {rel}  (file not found)")
            continue
        status = process(p)
        print(f"  OK    {rel}  -- {status}")


if __name__ == "__main__":
    main(sys.argv)
