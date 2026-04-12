# Usage Token Dedup — Design Spec

**Status:** Draft, pending review **Author:** Claude + Wes **Date:** 2026-04-12
**Related:** `docs/superpowers/plans/2026-04-12-usage-dedup-align-ccusage.md`

## Problem

`agentsview usage daily` over-counts Claude tokens — on the author's DB
(all-history):

| metric   | agentsview | ccusage | ground truth (raw files, deduped) |
| -------- | ---------- | ------- | --------------------------------- |
| input    | 35.06M     | 27.62M  | 27.58M                            |
| output   | **44.77M** | 15.06M  | **15.08M**                        |
| cache_cr | 1.14B      | 1.07B   | 1.06B                             |
| cache_rd | 25.43B     | 23.97B  | 23.75B                            |

Output tokens are ~3x over truth. ccusage matches ground truth within 1%.

**Ground truth method:** scan every `~/.claude/projects/**/*.jsonl` file, parse
each `assistant` entry with a `message.usage` block, dedup globally by the pair
`(message.id, requestId)`, sum. This is exactly what ccusage does (see
`apps/ccusage/src/data-loader.ts:530-540`).

**Why agentsview differs:** It only collapses *consecutive* streaming snapshots
within a single file (`collapseStreamingDuplicates` in
`internal/parser/claude.go:611-645`). It has no cross-file / cross-session
dedup. When the DAG-fork logic splits a file into multiple sessions and later
re-parses the file, the same API response can end up stored under both the main
session's row **and** a new fork session's row — query-side sums then double (or
triple, or more) count it. Observed concretely in
`/Users/wesm/.claude/projects/-Users-wesm-code-moneyflow/38788228-d413-476a-b58b-401785c8c779.jsonl`:
1 main + 77 fork sessions with 32 messages appearing in both main and the
largest fork.

**Also found:** `internal/pricing/fallback.go:17-22` ships `claude-opus-4-6` at
1/3 real Anthropic rates (5/25/6.25/0.50 vs correct 15/75/18.75/1.50). Unrelated
but worth fixing in the same branch.

## Goal

Make `agentsview usage daily --agent claude` match `ccusage daily` within ~1%
for the same window. Fix the Opus-4-6 fallback rates as a side quest.

**Non-goal:** Repair the DAG-fork staleness of individual rows in the `messages`
table. Query-time dedup makes it invisible for token sums (the user-observable
impact). Row cleanup is a separate, more invasive change.

## Approach Options

### Option A: New columns on `messages` table (recommended)

Add two TEXT columns — `claude_message_id` and `claude_request_id` — to the
`messages` table. Populate on parse. At query time, `GetDailyUsage` tracks a
`map[string]struct{}` of seen `"msgId:reqId"` keys and skips rows whose key has
already been counted. Rows where either key is empty (legacy data, non-Claude
agents) are always counted — same fallback as ccusage's `createUniqueHash`
returning `null`.

**Pros:**

- Matches ccusage's algorithm 1:1. Easy to reason about parity.
- Dedup decision is data-driven, not structural — survives any future parser
  changes that affect how sessions/forks get split.
- Columns are tiny (both usually \<40 bytes, empty for non-Claude), indexed only
  if we decide dedup needs to go into SQL (it doesn't — in-memory Set is fine
  for scan sizes we've seen).
- Keeps existing token_usage blob unchanged — no contract break for PG push or
  UI consumers.

**Cons:**

- Schema migration required (details below).
- `dataVersion` bump forces a resync on next open so existing rows get
  backfilled — a few minutes of work for users with large session histories.
- Two more columns in every message row (hundreds of thousands of rows on real
  DBs).

### Option B: Extend the `token_usage` JSON blob

Store `{"id": "msg_X", "requestId": "req_Y", "usage": {...}}` instead of just
`{...}`. No new columns. Dedup reads from the blob at query time.

**Pros:**

- No schema migration.

**Cons:**

- Changes the shape of `token_usage` from "raw Anthropic usage" to
  "agentsview-specific envelope". Any consumer that parses the column expecting
  Anthropic shape breaks (PG push copies the blob verbatim to a shared Postgres
  instance, and PG analytics queries parse its fields). We'd need every PG
  consumer to adopt the new shape in lockstep.
- Harder to add a SQL-side dedup index if we ever need one.
- Still needs a data version bump / resync to rewrite existing blobs in the new
  shape.
- No meaningful savings — still two strings stored per row.

### Option C: Filter by `relationship_type` in the query

Exclude `fork`, `subagent`, `continuation` from the usage query. One-line change
to `internal/db/usage.go`.

**Pros:**

- Trivial diff.

**Cons:**

- Does **not** match ccusage, which counts all of those (they're real API
  calls).
- Measured: even main-only sessions over-count on the author's DB (~1.76x output
  tokens over truth). DAG-fork staleness affects main rows too, not just fork
  rows — so filtering relationship type doesn't actually fix the bug.
- Drops legitimate work: if you resume or fork a session and get real new
  assistant responses, those tokens disappear from the report.

### Option D: Dedup at parse time by re-reading the source file

During sync, hold global state that skips inserting a row whose
`(msg.id, requestId)` has been seen before in this sync pass.

**Pros:**

- No new columns.

**Cons:**

- Breaks incremental sync. Cross-file dedup requires knowing about rows from
  *other* files when parsing *this* file — which means holding global state
  across the entire project tree per sync.
- Doesn't fix existing DBs without a resync.
- If two files legitimately share a message (rare but possible: orphaned data +
  current data), we lose the row structure rather than just the token sum.
  Messages and their tool calls become inconsistent.
- Harder to test; bugs in the global-state machinery cause data loss, not just
  wrong numbers.

### Option E: Re-read JSONL files at query time

Every `usage daily` invocation opens the relevant source files, re-dedups on the
fly.

**Pros:**

- No schema change, no migration.

**Cons:**

- Defeats the point of the sync pipeline. Slow for 6k+ files.
- Breaks the "orphaned data survives source deletion" guarantee — usage reports
  would suddenly exclude archived sessions whose source files were
  moved/deleted.
- `usage statusline` is called on every shell prompt redraw for users who wire
  it up; reading files on that hot path is unacceptable.

## Recommendation

**Option A.** It's the only option that (a) matches ccusage's algorithm exactly,
(b) preserves the persistent-archive guarantee, (c) stays on the warm path's
SQLite read model, and (d) leaves the `token_usage` blob contract intact for PG
push consumers.

## Schema Migration Details

Existing `messages` schema (`internal/db/schema.sql:30-48`):

```sql
CREATE TABLE IF NOT EXISTS messages (
    id             INTEGER PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal        INTEGER NOT NULL,
    -- ... 12 more columns ...
    token_usage TEXT NOT NULL DEFAULT '',
    context_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    has_context_tokens INTEGER NOT NULL DEFAULT 0,
    has_output_tokens INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ordinal)
);
```

**Proposed additions:**

```sql
    claude_message_id TEXT NOT NULL DEFAULT '',
    claude_request_id TEXT NOT NULL DEFAULT '',
```

**Type/nullability choice — `TEXT NOT NULL DEFAULT ''`:**

- Matches existing convention: `token_usage TEXT NOT NULL DEFAULT ''`,
  `model TEXT NOT NULL DEFAULT ''`.
- Alternative is `TEXT NULL` (semantically clearer for "not applicable"), but
  breaks convention. The default string "" is a sentinel for "no dedup key",
  treated as "count this row" by the query. Any comparison `msgId != ""` is
  sufficient.

**Migration for existing databases:**

- `internal/db/db.go:264-332` has a `migrateColumns` function that runs
  `ALTER TABLE ... ADD COLUMN` idempotently on every open. It checks
  `PRAGMA table_info` per column before running its DDL, so re-runs are cheap
  no-ops.
- Two entries get appended:
  ```go
  {"messages", "claude_message_id",
    "ALTER TABLE messages ADD COLUMN claude_message_id TEXT NOT NULL DEFAULT ''"},
  {"messages", "claude_request_id",
    "ALTER TABLE messages ADD COLUMN claude_request_id TEXT NOT NULL DEFAULT ''"},
  ```
- `ALTER TABLE ADD COLUMN` in SQLite with a NOT NULL + constant default is O(1)
  — SQLite does not rewrite existing rows. Metadata change only. Instant even on
  a 1.4 GB `sessions.db`.
- No DROP. No data loss. No FTS rebuild. Follows the project CLAUDE.md
  directive: "never drop, truncate, or recreate the database."

**Index strategy:** none. The query scans rows by date range and joins through
sessions; dedup happens in Go memory via `map[string]struct{}`. We measured:
current all-history scan is ~268k message rows, the map holds ~247k entries.
That's fine for a CLI pass; `usage statusline` already re-runs this per prompt
and is fast enough.

**PG mirror:** `internal/postgres/schema.go` has the equivalent migration
harness. Adding the same two columns keeps push sync consistent so an eventual
PG-side usage report can dedup the same way. This is in the plan but scoped as
"don't block on it" — SQLite is the primary consumer for `usage daily`.

## Data Version Bump — Is It Necessary?

**The question:** If we add the columns and start populating them for new
messages, do we need to force existing rows to get backfilled?

**Yes, partially.** Current DBs have ~268k existing rows with empty
`claude_message_id`/`claude_request_id`. Without backfill, every one of those
rows bypasses dedup (empty key = always counted). A `dataVersion` bump backfills
*the subset whose source `.jsonl` files still exist*. Rows whose source files
are gone (orphaned sessions — see limitation below) cannot be backfilled.

**Options for backfill:**

1. **Bump `dataVersion` from 8 to 9** — `NeedsResync()` returns true on next
   open, all CLI entry points (`main.go:245`, `usage.go:226`, `sync.go:111`)
   call `ResyncAll`, which builds a fresh DB next to the old one and swaps on
   success. Non-destructive (old DB kept until success). This is the existing,
   tested mechanism. **Recommended.**

1. **Manual one-time script** — `agentsview sync --full` already exists. User
   has to know to run it. Not automatic. Reject — too easy to miss.

1. **Lazy backfill on read** — have `usage daily` notice empty keys and re-parse
   files to fill them in. Works the first time, but needs to handle sessions
   whose source files are gone (orphaned data stays empty). Adds complexity to a
   hot query path. Reject.

1. **No backfill — live with wrong numbers until natural churn** — as messages
   age out of the 30-day default window, new deduped rows replace them.
   Eventually converges. Takes 30 days to get correct numbers. Reject — point of
   the fix is to be correct now.

**Resync cost:** On the author's DB (~6.8k files, ~20k sessions, 1.4 GB
`sessions.db`), a full resync takes ~60-90 seconds. Smaller DBs are
proportionally faster. The resync runs once per data version bump and survives
process kills (swap is atomic). Users are accustomed to this from previous bumps
(dataVersion has gone from 1 → 8 already).

**Recommendation:** bump to 9. It's the mechanism the codebase was designed
around for exactly this case.

### Unavoidable Limitation: Orphaned Sessions

`ResyncAll` rebuilds the DB by re-parsing files on disk, **then** copies
"orphaned" sessions (those whose source files are gone) from the old DB into the
new DB via `CopyOrphanedDataFrom` (`internal/sync/engine.go:819` →
`internal/db/orphaned.go:113-140`). The copy is column-probe driven: it only
copies columns that existed in the old schema. On an 8→9 upgrade, the old schema
has no `claude_message_id`/`claude_request_id` columns, so orphaned rows land in
the new DB with the default `''` for both — permanently empty, never deduped.

**Impact:** token counts for orphaned Claude sessions remain potentially
inflated. Users observe:

- Cross-session duplicates within a single *deleted* source file — not deduped.
- Duplicates between two orphaned files that shared a `(message.id, requestId)`
  — not deduped.
- Any dedup interaction *between* an orphaned session and a live session — not
  deduped (orphaned side has empty keys, so the dedup key lookup misses).

**Scope of impact on observed data:** the author's DB currently has zero
orphaned Claude sessions (every `file_path` in the DB still points to an
existing file). So the 8→9 bump will actually backfill 100% of rows on this
machine. The limitation is real and permanent, but in practice it only bites
users who have already archived / deleted Claude project files before upgrading.

**Forward compatibility:** add `claude_message_id` and `claude_request_id` to
the column probe list in `internal/db/orphaned.go` so *future* dataVersion bumps
can carry the values forward from the 9-era DB to a 10-era DB without loss. This
is a one-line change included in the plan.

**Not mitigated here:** writing a backfill pass that re-parses the current file
tree to retroactively fill dedup keys on orphaned rows that no longer have
sources. Can't be done without the source files; the data simply isn't
recoverable.

## Resolved Design Choices

1. **Column naming:** `claude_message_id` / `claude_request_id` — specific to
   Claude now; generalize later only if another agent exposes comparable
   identifiers.

1. **Storage shape:** two separate TEXT columns, not one composite hash. Keeps
   the individual fields available for UI display, debugging, and future PG-side
   work.

1. **Dedup scope:** agent-agnostic at the query level. Dedup activates whenever
   both keys are non-empty, which in practice only happens for Claude rows since
   only the Claude parser populates them.

1. **Opus-4-6 pricing fix:** same branch. One-line fix found in the same
   investigation; splitting it into its own PR would be churn.

1. **PG push parity:** deferred. The new columns will not flow to Postgres in
   this branch. SQLite-only fix. A follow-up is needed before enabling PG-side
   dedup — see "Follow-up Work" below.

1. **DAG-fork row staleness:** out of scope. Query-time dedup hides it for token
   sums. A separate project is needed if we want to purge the stale rows
   themselves (must handle pinned messages, subagent links, PG push state).

## Follow-up Work (Not in This Branch)

When we revisit PG parity:

- Add `claude_message_id` and `claude_request_id` columns to the PG `messages`
  schema with the same default.
- Extend `MessageTokenFingerprint` in `internal/db/messages.go:733-767` to hash
  the two new columns. Without this, PG push's fast-path will skip re-sending
  rows whose token usage looks unchanged, even if the dedup keys got backfilled
  after a `dataVersion` bump. Result would be: SQLite has dedup keys; PG
  silently stays empty.
- Mirror the same change in `internal/postgres/push.go:873-969` if PG has its
  own fingerprint.
- Extend `internal/db/orphaned.go` column probe (done in this branch for forward
  compat) and make sure the PG schema migration runs before the first
  post-upgrade push.
- Add PG-side dedup to whatever usage report reads through PostgreSQL (none
  exists today, so this is blocked on the reader being built).

## Risk Assessment

| risk                                                                                      | likelihood | impact                       | mitigation                                                                                                  |
| ----------------------------------------------------------------------------------------- | ---------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------- |
| Missed Scan callsite after column addition → runtime error                                | medium     | high (server crash on query) | Task 5 step 3 forces an `rg` sweep + build check before moving on                                           |
| Resync takes too long on very large DBs                                                   | low        | medium (user wait)           | Resync is interruptible and non-destructive; worst case user `Ctrl+C`s and reruns                           |
| PG push falls behind SQLite if Task 8 has a bug                                           | low        | low                          | PG side gracefully ignores columns it doesn't know about; worst case PG messages table has empty dedup cols |
| Dedup drops legitimate duplicate messages (same msgId+reqId but different real API calls) | near-zero  | low                          | Anthropic's msg.id is globally unique by design; impossible to share across real calls                      |
| `dataVersion` bump breaks an unrelated older migration                                    | near-zero  | high                         | Existing migration harness is idempotent; bump only affects whether a resync is triggered                   |
| Opus-4-6 pricing change silently surprises users who liked the "low" cost                 | near-zero  | low                          | This is a correctness fix; the "low" numbers were wrong                                                     |

## Alternative: "Do Nothing" Baseline

If we did nothing, the user-visible state:

- agentsview usage is wrong by 1.5-3x on output tokens.
- Costs in the statusline are wrong both directions: tokens inflated, but
  Opus-4-6 priced 3x low — they partially cancel for Opus-heavy workloads.
- Users switching from ccusage see divergent numbers and lose trust in
  agentsview's usage feature.

Not acceptable.

## Decision Requested

**Approve Option A** (new columns + dedup at query time + dataVersion bump +
Opus fallback fix)?

If yes, `docs/superpowers/plans/2026-04-12-usage-dedup-align-ccusage.md` has the
10-step implementation plan, ready to execute via subagent-driven development.

If you want a different option, say which and I'll rewrite the plan.
