#!/usr/bin/env python3
"""Check the two www/ couplings that fail in production and nowhere else.

Both of these ship green and break only once Cloudflare is serving the result:

1. The CSP in www/_headers pins a SHA-256 of every inline <script>. Edit one of
   those scripts without recomputing its hash and the browser refuses to run it,
   so the page paints unthemed. Nothing local sends the header, so nothing local
   notices.
2. Tailwind scans only the files named by @source in www/css/input.css. Add a page
   and forget the line and the build emits no classes for it: the stylesheet is
   still "fresh" by a rebuild-and-diff check, and the page ships unstyled.

Run it by hand any time: python3 .github/scripts/verify-site.py
"""

import base64
import hashlib
import pathlib
import re
import sys

WWW = pathlib.Path(__file__).resolve().parents[2] / "www"
SCRIPT_RE = re.compile(r"<script([^>]*)>(.*?)</script>", re.S)

failures: list[str] = []


def inline_scripts(html: str):
    """Yield the bodies of executable inline scripts only.

    A src= element is external and a type the browser will not execute (JSON-LD,
    say) is a data block; script-src gates neither, so neither needs a hash.
    """
    for attrs, body in SCRIPT_RE.findall(html):
        if "src=" in attrs or "ld+json" in attrs:
            continue
        yield body


pages = sorted(WWW.glob("*.html"))
if not pages:
    sys.exit("no HTML pages found under www/ - is this running from the repo?")

headers_path = WWW / "_headers"
if not headers_path.is_file():
    sys.exit("www/_headers is missing; the CSP hashes cannot be checked")
headers = headers_path.read_text(encoding="utf-8")

input_css = (WWW / "css" / "input.css").read_text(encoding="utf-8")
sources = set(re.findall(r'@source\s+"([^"]+)"', input_css))

for page in pages:
    html = page.read_text(encoding="utf-8")

    for body in inline_scripts(html):
        digest = base64.b64encode(hashlib.sha256(body.encode("utf-8")).digest()).decode()
        token = f"sha256-{digest}"
        if token not in headers:
            failures.append(
                f"{page.name}: an inline <script> is not allowed by the CSP.\n"
                f"    expected in www/_headers script-src: '{token}'\n"
                f"    fix: recompute with the command in www/_headers and paste the new hash."
            )

    # @source paths in input.css are relative to www/css/, so a page one level up
    # is written "../<name>".
    if f"../{page.name}" not in sources:
        failures.append(
            f"{page.name}: no @source line in www/css/input.css.\n"
            f'    fix: add \'@source "../{page.name}";\' and re-run npm run build:css,\n'
            f"    or Tailwind emits none of this page's classes and it ships unstyled."
        )

if failures:
    print("site checks FAILED:\n", file=sys.stderr)
    for f in failures:
        print(f"  - {f}\n", file=sys.stderr)
    sys.exit(1)

print(f"site checks passed: {len(pages)} pages, CSP hashes and @source lines all present.")
