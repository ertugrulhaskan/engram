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
import os
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


# rglob, not glob: a page in a subdirectory is still a page, and skipping it would
# make this script report success over exactly the file it failed to check.
pages = sorted(WWW.rglob("*.html"))
if not pages:
    sys.exit("no HTML pages found under www/ - is this running from the repo?")

headers_path = WWW / "_headers"
if not headers_path.is_file():
    sys.exit("www/_headers is missing; the CSP hashes cannot be checked")


def csp_values(text: str) -> list[str]:
    """Every Content-Security-Policy value in _headers, and nothing else.

    Deliberately not a substring search over the file. The comment block in
    _headers explains how to recompute a hash, so a hash pasted into a comment
    instead of into script-src would satisfy a naive `in` test while production
    still blocks the script - the exact failure this is here to catch.
    """
    values, in_rule = [], False
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if not line[0].isspace():  # a rule line, e.g. "/*"
            in_rule = True
            continue
        if in_rule and ":" in stripped:
            name, value = stripped.split(":", 1)
            if name.strip().lower() == "content-security-policy":
                values.append(value)
    return values


policies = csp_values(headers_path.read_text(encoding="utf-8"))
if not policies:
    sys.exit("www/_headers declares no Content-Security-Policy header")

input_css = (WWW / "css" / "input.css").read_text(encoding="utf-8")
sources = set(re.findall(r'@source\s+"([^"]+)"', input_css))

for page in pages:
    rel = page.relative_to(WWW).as_posix()
    html = page.read_text(encoding="utf-8")

    for body in inline_scripts(html):
        digest = base64.b64encode(hashlib.sha256(body.encode("utf-8")).digest()).decode()
        token = f"sha256-{digest}"
        if not any(token in policy for policy in policies):
            failures.append(
                f"{rel}: an inline <script> is not allowed by the CSP.\n"
                f"    expected in www/_headers script-src: '{token}'\n"
                f"    fix: recompute with the command in www/_headers and paste the new hash."
            )

    # @source paths in input.css are relative to www/css/, so derive the expected
    # spelling from the page's real location rather than assuming it sits in www/.
    expected = os.path.relpath(page, WWW / "css").replace(os.sep, "/")
    if expected not in sources:
        failures.append(
            f"{rel}: no @source line in www/css/input.css.\n"
            f'    fix: add \'@source "{expected}";\' and re-run npm run build:css,\n'
            f"    or Tailwind emits none of this page's classes and it ships unstyled."
        )

if failures:
    print("site checks FAILED:\n", file=sys.stderr)
    for f in failures:
        print(f"  - {f}\n", file=sys.stderr)
    sys.exit(1)

print(f"site checks passed: {len(pages)} pages, CSP hashes and @source lines all present.")
