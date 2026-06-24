# Security Policy

## Supported versions

engram is pre-1.0 and ships from `main`; security fixes land on the latest
release line only.

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅         |
| < 0.1   | ❌         |

## Reporting a vulnerability

**Please don't open a public issue for security problems.**

Report privately through GitHub's
[private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability):
on the repository, go to **Security → Report a vulnerability**. If that isn't
available to you, email the maintainer at **ertughaskan@gmail.com** with details
and, if possible, a minimal reproduction.

engram runs entirely locally: it reads files under `~/.claude/` and writes only
on an explicit edit/create/delete, and it has no network service. The most
relevant areas for a report are path handling, the `$EDITOR` / `claude`
subprocess handoff, and the `MEMORY.md` index rewrite. We'll acknowledge a valid
report as soon as we reasonably can and keep you updated on the fix.
