# v1.4.0 improvement batch

Selected from the post-v1.3.0 backlog: all quick wins, all UI/layout items, all
functionality items. Code health work (file splits, TUI flow test suite,
Windows signing) deferred by choice. Previous v1.3.0 pass is recorded in git
history (todo.md at 92ee261).

## TUI/UX batch

- [x] T2 Graph error formatting: parse OData error body in graph.do(), typed
      StatusError, "code: message" with "HTTP n status" fallback + excerpt
- [x] T1 Connection removal confirm: y/n prompt before removal; shift or
      invalidate syncSourceConn/syncTargetConn; fail-closed check at sync confirm
- [x] T5 Empty-filter hint: filterLine shows no-matches hint with esc-to-clear
- [x] T6 Browse loading spinner: browseLoading flag, spinner line, enter ignored
      while loading, cleared on backupsLoadedMsg/errMsg
- [x] T3 List sorting: s cycles default/name on browse, category policies, sync
      policies, restore pick; sort reorders post-filter visible slice only;
      reset on screen entry via resetFilter
- [x] T8 Sync step breadcrumb: "step N of 6" on the six sync wizard screens and
      on shared pick screens when in sync mode
- [x] T4 Export results: e writes timestamped JSON report (diff / restore /
      sync results) under backup root, transient "Exported to <path>" status
- [x] T7 Narrow terminals: width0 floor 40, dashboard stacks below 60 cols,
      footer hints truncated to width; width-sweep render test

## Backend batch

- [x] T10 Parallel creates: 4-worker indexed pool in syncer.Run (pattern from
      backup.Run); restore.Restore becomes an adapter over syncer.Run;
      per-item multi-step creates stay sequential inside policyops.Create
- [x] T11 Checksums: sidecar checksums.json (sha256, category-relative paths,
      real deduped filenames); store.Verify; headless `verify <dir>` subcommand
      (exit 0/1/2); optional v key on browse detail
- [x] T12 Headless `sync` subcommand: --from-backup | TENUVAULT_SOURCE_* env
      live source, --categories, --names, --prefix, --dry-run; target creds =
      AZURE_* like restore; refuse same-tenant sync without prefix
- [x] T13 Assignment group names: catalog NoAssignments flag replaces
      supportsAssignments list; groupResolver (cached, 403-degrades) in backup;
      backup-level groups.json sidecar; Group.Read.All added to delegated scopes

## Acceptance criteria (contract for evaluation)

### T2
- 403 with OData body surfaces "Authorization_RequestDenied: Insufficient
  privileges..." not raw JSON; non-JSON body surfaces "HTTP 500 Internal Server
  Error" plus short excerpt; 429/503/504 retry behavior unchanged
- StatusError carries the HTTP code for errors.As callers

### T1
- x arms a confirm prompt; y removes (config + conns persisted); n/esc/any
  other key cancels with no change; x on the add-tenant row does nothing
- Removing a connection before the sync source/target index shifts it; removing
  the selected one sets -1; sync confirm y with stale source/target refuses
  with an error status instead of running

### T5
- Zero-match filter renders: no matches for "query" (esc to clear); gone when
  matches exist

### T6
- Entering browse shows animated scanning line until load lands; stale rows
  never render; enter ignored while loading; errMsg clears the flag

### T3
- s toggles name-asc/default on all four screens with a visible sort indicator
- Composes with / filter; selections and mouse hits stay on the same items
- s typed while filter input is active goes into the query, not the sort
- Browse default order stays newest-first

### T8
- Six sync screens show correct step N of 6; shared pick screens only in sync
  mode; non-sync flows show no step text

### T4
- e on diff/restore/sync result screens writes valid timestamped JSON under the
  backup root and shows Exported-to status; failed items carry error text; sync
  partial-content flagged; unwritable root produces an error status, no file

### T7
- Every screen renders without panic at width 40; dashboard stacks below 60;
  footer line never exceeds render width; width under 30 yields 40-col output

### T10
- Concurrency observed (max in-flight between 2 and 4) on 8+ items; results in
  input order; cancel mid-run loses no results (unstarted items get
  context.Canceled); multi-part group policy child POST always after its create
- Existing restore package tests pass unchanged

### T11
- checksums.json hashes match a re-hash of every written file including
  dedup-renamed ones; verify distinguishes ok/mismatch/missing/extra/no-file;
  CLI exit codes 0/1/2 covered by tests

### T12
- Dry-run from backup dir prints planned items, exits 0, makes zero network
  calls; --names filters; missing source spec exits 2; README documents env vars

### T13
- groups.json written with cached single fetch per group id; 403 on /groups
  degrades: backup still succeeds, one warning line in backup.log, raw IDs kept
- The 6 no-assignment types pinned by a test; restore/sync never POST
  assignments (existing tests stay green)

### Whole batch
- gofmt clean; go vet clean; go test -race ./... green; golangci-lint 0 issues
- Independent code-reviewer agent evaluation against these criteria: PASS
- README updated for: sorting/export/verify/sync keys and subcommands, env
  vars, Group.Read.All scope note

## Review

Acceptance criteria results (2026-07-02):

1. All 12 features implemented and individually tested (see per-task criteria
   above); every criterion verified by the independent reviewer. PASS
2. gofmt clean, go vet clean, go test -race ./... green (11 packages),
   golangci-lint 0 issues. PASS
3. Independent feature-dev:code-reviewer evaluation of the full diff: first
   pass found one high-confidence regression (a mouse click on the Tenants
   screen while a removal confirmation was pending was swallowed as a fake
   keypress and could neither be a deliberate confirm nor a clean cancel).
   Fixed in mouse.go (click during pending confirm = dismiss + move cursor,
   never fires the row action) with regression test
   TestMouseClickDuringRemoveConfirm. Re-review verdict: PASS, no remaining
   high-confidence issues.
4. README updated: sort/export/verify keys, verify and sync subcommands with
   examples, TENUVAULT_SOURCE_* env vars, Group.Read.All scope note, backup
   format (checksums.json, groups.json). PASS

Notes:
- New tests added: 21 (graph error formatting, removal confirm incl. mouse,
  sort/filter composition, empty-filter hint, browse loading, breadcrumbs,
  exports, narrow-width sweep, syncer pool concurrency/order/cancel/multi-part,
  checksums round-trip, store verify matrix, CLI verify/sync exit codes,
  group resolver cache + 403 degrade, NoAssignments pin).
- Live-tenant verification of the new Graph call (GET /v1.0/groups/{id}
  ?$select=displayName) was not run this session; the endpoint shape matches
  the documented v1.0 groups API and degrades safely on 403/404. Suggested
  before release: one --assignments backup against the lab tenant.

Changes are uncommitted in the working tree (30 files, +2000/-290 approx),
ready for review and commit.
