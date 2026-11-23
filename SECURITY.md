# Security Policy

## Security Model

Hearth is a **local-first desktop application** with zero telemetry and no backend service:

- **Read-only scanning:** Discovering tools, projects, Docker contexts, Kubernetes configs, and AI agents never modifies files, installs packages, or runs project code.
- **Explicit plans & diffs:** Every mutating operation previews the exact `argv` commands, file diffs, administrator privilege requirements, and consequences before execution. Commands are never passed through a shell string.
- **Reversible file edits:** Shell startup files are modified inside `# >>> hearth >>>` / `# <<< hearth <<<` markers with timestamped backups and post-edit verification.
- **Secret handling:**
  - Environment variable values, MCP server secrets, SSH private keys, cloud tokens, and Kubernetes Secret values are never saved in Hearth's snapshot cache or diagnostic reports.
  - Sensitive values inspected on demand (container `Env`, Kubernetes Secret keys) are shown one value at a time for 30 seconds and never written to logs.
- **Built-in MCP server (`hearth mcp`):**
  - Read tools return strict field projections without environment values, MCP commands/args/URLs, instruction/hook bodies, or session transcripts.
  - `propose_fix` accepts only a hashed `issue_id` over a `127.0.0.1` loopback bridge authenticated by a per-session `0600` token. The desktop window always requires explicit human confirmation; AI agents cannot submit arbitrary commands or bypass confirmation.
- **Signed releases & updates:** Release artifacts include `SHA256SUMS.txt`, GitHub build provenance attestations, and Ed25519-signed auto-update manifests.

## Supported Versions

Security fixes are provided for the latest `v1.x` release.

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for suspected security vulnerabilities.

Instead, report privately using [GitHub Security Advisories](https://github.com/Blinks44/hearth/security/advisories/new) ("Report a vulnerability" in the repository's **Security** tab).

Include as much detail as possible:
- Hearth version, OS, and architecture
- Steps to reproduce or proof of concept
- Expected impact (e.g., command injection, secret disclosure, privilege escalation, MCP bridge bypass)

We aim to acknowledge reports within 72 hours and coordinate a fix and disclosure timeline with you.
