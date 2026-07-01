# Full codebase review and improvement pass

Goal: completely review the codebase and all TUI pages, fix bugs, improve what
exists, and add features that make TenuVault-TUI top-notch.

## Plan

- [x] Baseline: build, vet, tests (all green at d3246ad)
- [x] Parallel deep review by area:
  - [x] internal/tui (all pages: dashboard, backup, restore, compare, sync, auth form, connections)
  - [x] internal/graph, auth, connection, config, store (Graph client, auth flows, secret handling)
  - [x] internal/backup, restore, syncer, policyops, diff, catalog, jsonutil, cli (core operations)
- [x] Triage findings: fix confirmed bugs first, then high-value improvements
- [x] Add features that raise product quality (cancel running ops, sync load retry, paste, compare CLI)
- [x] Verify: build + vet + full test suite green, new behavior covered by tests
- [x] Independent evaluation by a separate code-reviewer agent
- [x] Document results in this file

## Fixed bugs

1. Graph client only retried 429; now also retries 503/504 with Retry-After
   (capped), plus bounded transport-error retries for GET only (POST/PATCH are
   never retried on transport errors to avoid double-creating policies).
2. /organization smoke test moved from beta to v1.0 (verified live via Lokka:
   GET /v1.0/organization?$select=id,displayName,verifiedDomains returns the
   exact shape the code parses).
3. SanitizeFilename and condense() truncated on byte boundaries and could split
   multi-byte UTF-8 runes (umlauts/CJK policy names); now rune-safe. trunc() in
   the TUI likewise.
4. Backup disk-write failures were silently counted as generic warnings; now
   recorded in the category error, written to backup.log, and emitted as events.
5. "?" was globally intercepted for help and could not be typed into the client
   secret field.
6. No paste support in the auth form; bracketed paste now inserts into the
   focused field (control characters stripped).
7. syncMode leaked from an abandoned sync flow into restore started from browse
   detail, silently rerouting restore into the sync target picker.
8. Removing a connection listed before the active source silently switched the
   active source tenant (sourceIdx not decremented).
9. Sync policy load errors were swallowed and shown as "No policies of this
   type in the source tenant"; now surfaced with a retry ('r' or reopen).
10. Connect results arriving after the user pressed esc on the connecting
    screen yanked them to the dashboard/error screen; now generation-guarded.

## Features added

- Cancel a running backup (x/esc) without quitting; cancelled backups keep what
  was written and finalize metadata with Status "Cancelled" instead of leaving
  an orphaned folder without a manifest. x also cancels running restore/sync.
- Headless `tenuvault compare <older> <newer>` subcommand for CI drift gates
  (exit 0 no drift, 1 drift, 2 error). README updated.
- Retention days bounded at 3650; esc from connections screen when not
  connected returns to sign-in; long tenant names truncated in the header.

## Acceptance criteria

1. go build ./..., go vet ./..., go test ./... all pass
2. Every confirmed bug found in review is fixed or explicitly documented as deferred
3. At least the top improvement/feature items are implemented with tests where testable
4. Any Graph API endpoint/payload change is verified against Graph docs (msgraph skill) before shipping
5. A separate code-reviewer agent signs off on the final diff with no high-severity findings

## Review section

Acceptance criteria results (2026-07-01):

1. go build ./..., go vet ./..., go test ./... all pass; gofmt clean; go test
   -race clean on tui/graph/backup. PASS
2. All 10 confirmed review bugs fixed (see "Fixed bugs" above); no findings
   deferred. Reviewer improvement ideas deliberately not done in this pass:
   "/" list filter and concurrent per-item backup fetches (both offered as
   spin-off tasks), backup checksums/resume, dashboard warning indicator.
3. Features implemented with tests where testable: run cancellation
   (TestRunCancelledKeepsPartialBackup), retry logic (3 graph client tests),
   rune-safe filenames (TestSanitizeFilenameRuneBoundary), sourceIdx fix
   (TestRemoveEarlierTenantKeepsSource), syncMode reset
   (TestRestoreFromBrowseDetailClearsSyncMode). PASS
4. Graph change (/organization beta -> v1.0) verified end-to-end live via
   Lokka MCP against tenant ffc10f05: response shape matches the parser. PASS
5. Independent feature-dev:code-reviewer evaluation of the full diff:
   verdict PASS, zero high or medium severity findings. It traced the
   generation guards, run/ui context split, cancellation-to-metadata path,
   retry bounds, and value-receiver mutation semantics. PASS

Changes are uncommitted in the working tree (16 files, +421/-56), ready for
review and commit.
