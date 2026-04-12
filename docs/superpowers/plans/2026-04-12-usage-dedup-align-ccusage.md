# Usage Token Dedup — Align with ccusage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `agentsview usage daily` match ccusage's numbers for Claude by
deduplicating assistant messages by `(message.id, requestId)` across all
sessions/files, and fix a related Opus-4-6 pricing bug.

**Architecture:** ccusage collapses duplicate usage entries globally via a
`${messageId}:${requestId}` hash set (see
`~/code/ccusage/apps/ccusage/src/data-loader.ts:530-540`). agentsview currently
only collapses *consecutive* streaming snapshots within a single file
(`collapseStreamingDuplicates`), so cross-session duplicates from DAG forks,
session resumes, and incremental re-parses inflate its token sums (all-history
output: 44.7M agentsview vs 15.1M ccusage vs 15.1M raw-file ground truth on the
author's DB). Fix by (a) extracting Claude's `message.id` and `requestId` on
parse, (b) persisting them as nullable columns on the `messages` table, (c)
deduping by those keys in `GetDailyUsage`, (d) bumping `dataVersion` so existing
databases re-sync and backfill. Same dedup rules as ccusage: only dedup when
BOTH keys are present; count non-keyed entries once (matches `createUniqueHash`
returning `null`).

**Tech Stack:** Go 1.24+, SQLite (WAL + FTS5 via `mattn/go-sqlite3`),
`tidwall/gjson` for JSON parsing, optional PostgreSQL push sync.

**Non-goals for this plan:**

- Reconciling the DAG-fork message-row staleness bug (stale rows in `messages`
  table when forks appear after initial parse). Dedup at query time makes this
  invisible for token sums, which is the reported user impact. Row-level cleanup
  is a separate concern.
- **PostgreSQL push sync parity.** Deferred. When revisiting PG: add the two
  columns to PG schema, mirror in push INSERT, **and** update
  `MessageTokenFingerprint` (`internal/db/messages.go:733-767`) to include them
  — otherwise push's fast-path will skip rows whose usage looks unchanged even
  after backfill, leaving PG silently empty on the dedup keys. See spec's
  "Follow-up Work" section.
- **Backfilling dedup keys on orphaned sessions** (sessions whose source
  `.jsonl` files are gone at upgrade time). Not recoverable without source
  files. See spec's "Unavoidable Limitation" section. Forward-compat for
  *future* dataVersion bumps is handled by Task 8.

______________________________________________________________________

## File Structure

**New files:** none.

**Modified files:**

| Path                                    | Responsibility                                                                                                                   |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `internal/pricing/fallback.go`          | Fix hard-coded Opus-4-6 rates to match real Anthropic pricing.                                                                   |
| `internal/parser/types.go`              | Add `ClaudeMessageID` + `ClaudeRequestID` fields on `ParsedMessage`.                                                             |
| `internal/parser/claude.go`             | Populate the new fields in `extractClaudeTokenFields`.                                                                           |
| `internal/parser/claude_parser_test.go` | Verify parser extracts `message.id` + `requestId`.                                                                               |
| `internal/db/schema.sql`                | Add `claude_message_id TEXT` + `claude_request_id TEXT` columns to `messages`.                                                   |
| `internal/db/db.go`                     | Migration for existing DBs; bump `dataVersion`.                                                                                  |
| `internal/db/messages.go`               | Add fields to `Message` struct; update `insertMessageCols` + `insertMessagesTx`.                                                 |
| `internal/db/usage.go`                  | Load new columns; dedup by `(msg_id, req_id)` struct-key set in `GetDailyUsage`; stable `ORDER BY` for deterministic first-wins. |
| `internal/db/usage_test.go`             | Unit tests covering cross-session dup scenarios.                                                                                 |
| `internal/sync/engine.go`               | Pass new fields through `toDBMessages`.                                                                                          |
| `internal/db/orphaned.go`               | Extend column probe list so future dataVersion bumps carry dedup keys forward from orphaned sessions.                            |

______________________________________________________________________

## Task 1: Fix Opus-4-6 fallback pricing

**Why:** `internal/pricing/fallback.go:17-22` ships claude-opus-4-6 at 1/3 the
real Anthropic rates (5/25/6.25/0.50 instead of 15/75/18.75/1.50). Compare lines
71-77 (`claude-opus-4-20250514`) for the correct Opus tier values. This causes
silent under-costing whenever the LiteLLM fetch fails and offline fallback kicks
in.

**Files:**

- Modify: `internal/pricing/fallback.go:16-22`

- Test: `internal/pricing/fallback_test.go` (create if missing)

- [ ] **Step 1: Check if a fallback test file exists**

Run: `ls internal/pricing/fallback_test.go 2>&1`

Expected: either file exists or `No such file or directory`. If it doesn't
exist, create it in the next step; otherwise add a new test function.

- [ ] **Step 2: Write a failing test pinning correct Opus-4-6 rates**

Create `internal/pricing/fallback_test.go` (or append to an existing one) with:

```go
package pricing

import "testing"

func TestFallbackPricing_Opus46Rates(t *testing.T) {
	prices := FallbackPricing()
	var got *ModelPricing
	for i := range prices {
		if prices[i].ModelPattern == "claude-opus-4-6" {
			got = &prices[i]
			break
		}
	}
	if got == nil {
		t.Fatal("claude-opus-4-6 entry missing from FallbackPricing")
	}

	// Source: https://www.anthropic.com/pricing — Opus tier.
	want := ModelPricing{
		ModelPattern:         "claude-opus-4-6",
		InputPerMTok:         15.0,
		OutputPerMTok:        75.0,
		CacheCreationPerMTok: 18.75,
		CacheReadPerMTok:     1.50,
	}
	if *got != want {
		t.Errorf("claude-opus-4-6 pricing = %+v, want %+v", *got, want)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/pricing/ -run TestFallbackPricing_Opus46Rates -v`
Expected: FAIL with a diff showing input=5 vs 15, output=25 vs 75, etc.

- [ ] **Step 4: Apply the fix**

Edit `internal/pricing/fallback.go` lines 17-22 to:

```go
		{
			ModelPattern:         "claude-opus-4-6",
			InputPerMTok:         15.0,
			OutputPerMTok:        75.0,
			CacheCreationPerMTok: 18.75,
			CacheReadPerMTok:     1.50,
		},
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/pricing/ -run TestFallbackPricing_Opus46Rates -v`
Expected: PASS.

- [ ] **Step 6: Run the rest of the pricing package tests**

Run: `go test ./internal/pricing/ -v` Expected: all PASS (no regressions in
existing litellm tests).

- [ ] **Step 7: Commit**

```bash
git add internal/pricing/fallback.go internal/pricing/fallback_test.go
git commit -m "fix(pricing): correct claude-opus-4-6 fallback rates

Fallback table shipped Opus-4-6 at 1/3 the real Anthropic rates
(5/25/6.25/0.50), causing offline cost reports to silently
under-count Opus usage. Restore 15/75/18.75/1.50 to match the
claude-opus-4-20250514 entry and the live LiteLLM data."
```

______________________________________________________________________

## Task 2: Add Claude-specific dedup fields on ParsedMessage

**Why:** ccusage dedups by `${message.id}:${requestId}`. agentsview's
`ParsedMessage` currently has no slot for these fields
(`internal/parser/types.go:446-468`) — they are only transiently present inside
the `TokenUsage` raw JSON blob, which contains only `message.usage` and not
`message.id` or top-level `requestId`. Add them as explicit fields so downstream
layers can see them.

**Files:**

- Modify: `internal/parser/types.go:446-468`

- [ ] **Step 1: Add fields to `ParsedMessage`**

Edit `internal/parser/types.go` lines 456-464 (the `Model`, `TokenUsage`, …
block) to:

```go
	Model            string
	TokenUsage       json.RawMessage
	ContextTokens    int
	OutputTokens     int
	HasContextTokens bool
	HasOutputTokens  bool

	// ClaudeMessageID and ClaudeRequestID hold the provider's
	// per-response identifiers. Used for cross-file / cross-session
	// deduplication when summing token usage, matching ccusage's
	// `${messageId}:${requestId}` hash. Only populated by the
	// Claude parser; empty for all other agents.
	ClaudeMessageID string
	ClaudeRequestID string
```

- [ ] **Step 2: Verify the package still compiles**

Run: `go build ./internal/parser/...` Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/parser/types.go
git commit -m "feat(parser): add ClaudeMessageID and ClaudeRequestID on ParsedMessage

Adds dedup-key slots that the Claude parser will populate in the
next commit and that db/usage.go will consume for cross-session
token deduplication aligned with ccusage."
```

______________________________________________________________________

## Task 3: Populate the new fields from Claude JSONL

**Files:**

- Modify: `internal/parser/claude.go:748-775` (`extractClaudeTokenFields`)

- Test: `internal/parser/claude_parser_test.go` — add a new test function

- [ ] **Step 1: Write the failing parser test**

Append to `internal/parser/claude_parser_test.go`:

```go
func TestParseClaudeSession_ExtractsMessageIDAndRequestID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess-1.jsonl")
	// Single assistant line with usage + id + requestId.
	line := `{"type":"assistant","uuid":"u1","parentUuid":"",` +
		`"timestamp":"2026-04-10T10:00:00.000Z",` +
		`"requestId":"req_01ABC",` +
		`"message":{"id":"msg_01XYZ","model":"claude-opus-4-6",` +
		`"content":[{"type":"text","text":"hi"}],` +
		`"usage":{"input_tokens":10,"output_tokens":20,` +
		`"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	results, err := ParseClaudeSession(path, "proj", "m")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	msgs := results[0].Messages
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	m := msgs[0]
	if m.ClaudeMessageID != "msg_01XYZ" {
		t.Errorf("ClaudeMessageID = %q, want msg_01XYZ", m.ClaudeMessageID)
	}
	if m.ClaudeRequestID != "req_01ABC" {
		t.Errorf("ClaudeRequestID = %q, want req_01ABC", m.ClaudeRequestID)
	}
	if m.OutputTokens != 20 {
		t.Errorf("OutputTokens = %d, want 20", m.OutputTokens)
	}
}
```

Ensure the imports include `"os"`, `"path/filepath"`, `"testing"`. If they are
already present at the top of the file you do not need to add them again.

- [ ] **Step 2: Run the test to verify it fails**

Run:
`go test -tags fts5 ./internal/parser/ -run TestParseClaudeSession_ExtractsMessageIDAndRequestID -v`
Expected: FAIL with `ClaudeMessageID = "", want msg_01XYZ`.

- [ ] **Step 3: Implement extraction**

Edit `internal/parser/claude.go` lines 748-775 (`extractClaudeTokenFields`).
Replace the function body with:

```go
// extractClaudeTokenFields populates Model, TokenUsage,
// ContextTokens, OutputTokens, ClaudeMessageID, and
// ClaudeRequestID on a ParsedMessage from a Claude JSONL line.
// Used by both full and incremental parsing paths.
func extractClaudeTokenFields(msg *ParsedMessage, line string) {
	msg.Model = gjson.Get(line, "message.model").String()
	msg.ClaudeMessageID = gjson.Get(line, "message.id").String()
	msg.ClaudeRequestID = gjson.Get(line, "requestId").String()

	usageResult := gjson.Get(line, "message.usage")
	if usageResult.Exists() {
		msg.TokenUsage = json.RawMessage(usageResult.Raw)
		msg.HasOutputTokens = usageResult.Get("output_tokens").Exists()
		msg.HasContextTokens = usageResult.Get("input_tokens").Exists() ||
			usageResult.Get("cache_creation_input_tokens").Exists() ||
			usageResult.Get("cache_read_input_tokens").Exists()

		input := int(usageResult.Get("input_tokens").Int())
		cacheCreation := int(usageResult.Get(
			"cache_creation_input_tokens",
		).Int())
		cacheRead := int(usageResult.Get(
			"cache_read_input_tokens",
		).Int())
		msg.OutputTokens = int(usageResult.Get(
			"output_tokens",
		).Int())
		msg.ContextTokens = input + cacheCreation + cacheRead
	}
}
```

- [ ] **Step 4: Run the parser test to verify it passes**

Run:
`go test -tags fts5 ./internal/parser/ -run TestParseClaudeSession_ExtractsMessageIDAndRequestID -v`
Expected: PASS.

- [ ] **Step 5: Run the full parser package test suite to catch regressions**

Run: `go test -tags fts5 ./internal/parser/ -v` Expected: all existing tests
PASS. If any test fails because its fixture line has a `requestId`/`message.id`
that are now populated into `ClaudeMessageID`/`ClaudeRequestID`, update the
expected values — do **not** revert the fix.

- [ ] **Step 6: Commit**

```bash
git add internal/parser/claude.go internal/parser/claude_parser_test.go
git commit -m "feat(parser): extract Claude message.id and requestId

Captures the per-response identifiers needed to deduplicate token
usage across files and DAG-fork sessions, matching ccusage's
\`\${messageId}:\${requestId}\` hash strategy."
```

______________________________________________________________________

## Task 4: Schema + migration for the new message columns

**Why:** The `messages` table needs two new optional columns. Existing databases
get them via `migrateColumns` (`internal/db/db.go:264-332`). Per project
CLAUDE.md: never drop/recreate — ALTER TABLE only. Existing rows will have empty
strings, which the dedup pass treats as "no dedup key, count once" (correct
fallback for legacy rows until the data version bump triggers resync).

**Files:**

- Modify: `internal/db/schema.sql:30-48`

- Modify: `internal/db/db.go:264-332` (append migration entries)

- [ ] **Step 1: Add columns to schema.sql**

Edit `internal/db/schema.sql` lines 30-48. Replace the `messages` table block
with:

```sql
-- Messages table with ordinal for efficient range queries
CREATE TABLE IF NOT EXISTS messages (
    id             INTEGER PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal        INTEGER NOT NULL,
    role           TEXT NOT NULL,
    content        TEXT NOT NULL,
    timestamp      TEXT,
    has_thinking   INTEGER NOT NULL DEFAULT 0,
    has_tool_use   INTEGER NOT NULL DEFAULT 0,
    content_length INTEGER NOT NULL DEFAULT 0,
    is_system      INTEGER NOT NULL DEFAULT 0,
    model TEXT NOT NULL DEFAULT '',
    token_usage TEXT NOT NULL DEFAULT '',
    context_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    has_context_tokens INTEGER NOT NULL DEFAULT 0,
    has_output_tokens INTEGER NOT NULL DEFAULT 0,
    claude_message_id TEXT NOT NULL DEFAULT '',
    claude_request_id TEXT NOT NULL DEFAULT '',
    UNIQUE(session_id, ordinal)
);
```

- [ ] **Step 2: Add migration entries for existing DBs**

Edit `internal/db/db.go` around line 308 (right after the `has_output_tokens`
migration). Append two more entries to the `migrations` slice, before the
`sessions`-related migrations that follow:

```go
		{
			"messages", "claude_message_id",
			"ALTER TABLE messages ADD COLUMN claude_message_id TEXT NOT NULL DEFAULT ''",
		},
		{
			"messages", "claude_request_id",
			"ALTER TABLE messages ADD COLUMN claude_request_id TEXT NOT NULL DEFAULT ''",
		},
```

- [ ] **Step 3: Verify schema compiles/applies by running DB open tests**

Run: `go test -tags fts5 ./internal/db/ -run TestOpen -v` Expected: all
`TestOpen*` tests PASS. The new columns should appear on fresh DBs and be
backfilled with empty strings on existing DBs via the migration.

- [ ] **Step 4: Commit**

```bash
git add internal/db/schema.sql internal/db/db.go
git commit -m "feat(db): add claude_message_id and claude_request_id columns

New dedup-key columns on the messages table. Non-destructive
ALTER TABLE migration for existing databases; columns default to
empty string which the query layer treats as \"no dedup key\"."
```

______________________________________________________________________

## Task 5: Plumb the new columns through the db layer

**Why:** The `db.Message` struct, the insert column list, and the prepared
statement parameter count all need to learn about the new columns. Without this,
parser data never reaches the DB.

**Files:**

- Modify: `internal/db/messages.go:15-27` (column constants)

- Modify: `internal/db/messages.go:74-94` (`Message` struct)

- Modify: `internal/db/messages.go:176-211` (`insertMessagesTx`)

- [ ] **Step 1: Extend column constants and struct fields**

Edit `internal/db/messages.go` lines 15-27:

```go
const (
	selectMessageCols = `id, session_id, ordinal, role, content,
		timestamp, has_thinking, has_tool_use, content_length,
		is_system,
		model, token_usage, context_tokens, output_tokens,
		has_context_tokens, has_output_tokens,
		claude_message_id, claude_request_id`

	insertMessageCols = `session_id, ordinal, role, content,
		timestamp, has_thinking, has_tool_use, content_length,
		is_system,
		model, token_usage, context_tokens, output_tokens,
		has_context_tokens, has_output_tokens,
		claude_message_id, claude_request_id`
```

Then edit the `Message` struct around line 74-94 to add the two fields just
after `HasOutputTokens`:

```go
// Message represents a row in the messages table.
type Message struct {
	ID               int64           `json:"id"`
	SessionID        string          `json:"session_id"`
	Ordinal          int             `json:"ordinal"`
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	Timestamp        string          `json:"timestamp"`
	HasThinking      bool            `json:"has_thinking"`
	HasToolUse       bool            `json:"has_tool_use"`
	ContentLength    int             `json:"content_length"`
	Model            string          `json:"model"`
	TokenUsage       json.RawMessage `json:"token_usage,omitempty"`
	ContextTokens    int             `json:"context_tokens"`
	OutputTokens     int             `json:"output_tokens"`
	HasContextTokens bool            `json:"has_context_tokens"`
	HasOutputTokens  bool            `json:"has_output_tokens"`
	// ClaudeMessageID and ClaudeRequestID are empty for non-Claude
	// agents and for legacy rows prior to the dedup-columns migration.
	ClaudeMessageID  string          `json:"claude_message_id,omitempty"`
	ClaudeRequestID  string          `json:"claude_request_id,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	ToolResults      []ToolResult    `json:"-"`
	IsSystem         bool            `json:"is_system"`
}
```

- [ ] **Step 2: Update `insertMessagesTx` to bind the new columns**

Edit `internal/db/messages.go:176-211`. Update the VALUES tuple to have 17
placeholders instead of 15 and pass the new fields:

```go
// insertMessagesTx batch-inserts messages within an existing
// transaction. Returns a slice of message IDs parallel to the
// input msgs slice. The caller must hold db.mu.
func (db *DB) insertMessagesTx(
	tx *sql.Tx, msgs []Message,
) ([]int64, error) {
	stmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO messages (%s)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, insertMessageCols))
	if err != nil {
		return nil, fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	ids := make([]int64, len(msgs))
	for i, m := range msgs {
		res, err := stmt.Exec(
			m.SessionID, m.Ordinal, m.Role, m.Content,
			m.Timestamp, m.HasThinking, m.HasToolUse,
			m.ContentLength, m.IsSystem,
			m.Model, string(m.TokenUsage),
			m.ContextTokens, m.OutputTokens,
			m.HasContextTokens, m.HasOutputTokens,
			m.ClaudeMessageID, m.ClaudeRequestID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"inserting message ord=%d: %w", m.Ordinal, err,
			)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf(
				"last insert id ord=%d: %w", m.Ordinal, err,
			)
		}
		ids[i] = id
	}
	return ids, nil
}
```

- [ ] **Step 3: Check every caller of `selectMessageCols` for SELECT/Scan that
  needs updating**

Run: `rg -n "selectMessageCols|rowsToMessages|scanMessage" internal/db` For each
hit, verify the Scan call reads all columns listed in `selectMessageCols`. You
will need to add two more destination pointers
(`&m.ClaudeMessageID, &m.ClaudeRequestID`) to each scanner. Typical locations
include `listMessagesByFilter`, `scanMessages`, etc. — there may be several;
update every one. Do not skip any: an out-of-sync Scan call will return a
runtime `sql: expected N destinations` error on the first query after the
migration, which can silently break the server.

- [ ] **Step 4: Build the db package**

Run: `go build ./internal/db/...` Expected: no errors. If build fails on Scan
call-sites you missed in Step 3, add the new destinations and rebuild.

- [ ] **Step 5: Run the db package tests**

Run: `go test -tags fts5 ./internal/db/ -v` Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/messages.go
git commit -m "feat(db): persist Claude message.id + requestId on messages

Plumbs the new dedup-key columns through Message, the SELECT
column list, and insertMessagesTx so parser output reaches the
database. Callers of selectMessageCols updated to scan the
additional fields."
```

______________________________________________________________________

## Task 6: Pass the new fields through the sync engine

**Files:**

- Modify: `internal/sync/engine.go:2580-2609` (`toDBMessages`)

- [ ] **Step 1: Extend `toDBMessages`**

Edit `internal/sync/engine.go` lines 2580-2609:

```go
// toDBMessages converts parsed messages to db.Message rows
// with tool-result pairing and filtering applied.
func toDBMessages(pw pendingWrite, blocked map[string]bool) []db.Message {
	msgs := make([]db.Message, len(pw.msgs))
	for i, m := range pw.msgs {
		hasCtx, hasOut := m.TokenPresence()
		msgs[i] = db.Message{
			SessionID:        pw.sess.ID,
			Ordinal:          m.Ordinal,
			Role:             string(m.Role),
			Content:          m.Content,
			Timestamp:        timeutil.Format(m.Timestamp),
			HasThinking:      m.HasThinking,
			HasToolUse:       m.HasToolUse,
			ContentLength:    m.ContentLength,
			IsSystem:         m.IsSystem,
			Model:            m.Model,
			TokenUsage:       m.TokenUsage,
			ContextTokens:    m.ContextTokens,
			OutputTokens:     m.OutputTokens,
			HasContextTokens: hasCtx,
			HasOutputTokens:  hasOut,
			ClaudeMessageID:  m.ClaudeMessageID,
			ClaudeRequestID:  m.ClaudeRequestID,
			ToolCalls: convertToolCalls(
				pw.sess.ID, m.ToolCalls,
			),
			ToolResults: convertToolResults(m.ToolResults),
		}
	}
	return pairAndFilter(msgs, blocked)
}
```

- [ ] **Step 2: Build the sync package**

Run: `go build ./internal/sync/...` Expected: no errors.

- [ ] **Step 3: Run the sync engine tests**

Run: `go test -tags fts5 ./internal/sync/ -v` Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/sync/engine.go
git commit -m "feat(sync): forward Claude dedup keys into db.Message"
```

______________________________________________________________________

## Task 7: Dedup in `GetDailyUsage`

**Why:** This is the actual behavior change users will observe. With the columns
in place, `GetDailyUsage` can mirror ccusage's loader: track a
`map[dedupKey]struct{}` of seen `(msgID, reqID)` pairs, skip rows already seen,
and count rows with missing keys every time — same rules as `createUniqueHash`
in `ccusage/apps/ccusage/src/data-loader.ts:530-540`.

**Implementation notes (review feedback):**

- Use a struct key (`type dedupKey struct{ msgID, reqID string }`), not a
  concatenated `"msgID:reqID"` string. Avoids an extra allocation per row, stays
  type-safe, and can't collide if a literal `:` ever shows up in one of the IDs.
- Add a deterministic `ORDER BY` to the scan query so "first row wins" stays
  stable across runs. Otherwise SQLite is free to return rows in physical
  storage order, which changes under VACUUM/merges. Use
  `ORDER BY COALESCE(m.timestamp, s.started_at) ASC, m.session_id ASC, m.ordinal ASC`.
  The semantic outcome (token totals) is identical either way because duplicates
  share the same usage — but tests become reproducible and debugging is easier.

**Files:**

- Modify: `internal/db/usage.go:139-249` (query + accumulator)

- Test: `internal/db/usage_test.go` — add a dedup test

- [ ] **Step 1: Write the failing dedup test**

Append to `internal/db/usage_test.go`:

```go
func TestGetDailyUsage_DedupesByClaudeMessageAndRequestID(t *testing.T) {
	d := testDB(t)

	// Seed pricing so cost math is stable.
	if err := d.UpsertModelPricing([]ModelPricing{{
		ModelPattern:         "claude-opus-4-6",
		InputPerMTok:         15.0,
		OutputPerMTok:        75.0,
		CacheCreationPerMTok: 18.75,
		CacheReadPerMTok:     1.50,
	}}); err != nil {
		t.Fatalf("seed pricing: %v", err)
	}

	// Two sessions that share the same (msg_id, req_id) for one
	// of the messages — this simulates agentsview's DAG-fork
	// bug where the same Claude API response is stored under
	// both a main session and a fork session.
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.getWriter().Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}

	mustExec(`INSERT INTO sessions (id, project, machine, agent, started_at, ended_at)
	          VALUES (?, ?, 'local', 'claude', ?, ?)`,
		"s-main", "proj",
		"2026-04-10T10:00:00Z", "2026-04-10T10:05:00Z")
	mustExec(`INSERT INTO sessions (id, project, machine, agent, started_at, ended_at, parent_session_id, relationship_type)
	          VALUES (?, ?, 'local', 'claude', ?, ?, 's-main', 'fork')`,
		"s-fork", "proj",
		"2026-04-10T10:01:00Z", "2026-04-10T10:06:00Z")

	// Shared duplicate message lives in BOTH sessions.
	shared := `{"input_tokens":100,"output_tokens":500,` +
		`"cache_creation_input_tokens":1000,` +
		`"cache_read_input_tokens":50000}`
	// Unique message — only in fork.
	unique := `{"input_tokens":20,"output_tokens":80,` +
		`"cache_creation_input_tokens":200,` +
		`"cache_read_input_tokens":5000}`

	for _, row := range []struct {
		sid, ts, usage, mid, rid string
		ord                      int
	}{
		{"s-main", "2026-04-10T10:02:00Z", shared, "msg_dup", "req_dup", 0},
		{"s-fork", "2026-04-10T10:02:00Z", shared, "msg_dup", "req_dup", 0},
		{"s-fork", "2026-04-10T10:03:00Z", unique, "msg_uniq", "req_uniq", 1},
	} {
		mustExec(`INSERT INTO messages
			(session_id, ordinal, role, content, timestamp,
			 model, token_usage,
			 claude_message_id, claude_request_id,
			 has_output_tokens, has_context_tokens)
			VALUES (?, ?, 'assistant', '', ?, 'claude-opus-4-6', ?, ?, ?, 1, 1)`,
			row.sid, row.ord, row.ts, row.usage, row.mid, row.rid)
	}

	result, err := d.GetDailyUsage(context.Background(), UsageFilter{
		From:     "2026-04-10",
		To:       "2026-04-10",
		Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("GetDailyUsage: %v", err)
	}

	if len(result.Daily) != 1 {
		t.Fatalf("daily entries = %d, want 1", len(result.Daily))
	}
	day := result.Daily[0]

	// Shared counted ONCE + unique added:
	//   input  = 100 + 20    = 120
	//   output = 500 + 80    = 580
	//   cr     = 1000 + 200  = 1200
	//   rd     = 50000 + 5000 = 55000
	if day.InputTokens != 120 {
		t.Errorf("input = %d, want 120 (shared counted once + unique)", day.InputTokens)
	}
	if day.OutputTokens != 580 {
		t.Errorf("output = %d, want 580", day.OutputTokens)
	}
	if day.CacheCreationTokens != 1200 {
		t.Errorf("cache_cr = %d, want 1200", day.CacheCreationTokens)
	}
	if day.CacheReadTokens != 55000 {
		t.Errorf("cache_rd = %d, want 55000", day.CacheReadTokens)
	}
}

func TestGetDailyUsage_MissingDedupKeysCountedEveryTime(t *testing.T) {
	d := testDB(t)
	if err := d.UpsertModelPricing([]ModelPricing{{
		ModelPattern: "claude-opus-4-6",
		OutputPerMTok: 75.0,
	}}); err != nil {
		t.Fatalf("seed pricing: %v", err)
	}

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.getWriter().Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO sessions (id, project, machine, agent, started_at, ended_at)
	          VALUES ('s1', 'proj', 'local', 'claude', ?, ?)`,
		"2026-04-10T10:00:00Z", "2026-04-10T10:05:00Z")

	usage := `{"input_tokens":0,"output_tokens":10,` +
		`"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`
	// Two messages with EMPTY dedup keys — must both be counted,
	// matching ccusage's null-hash fallback.
	for _, ord := range []int{0, 1} {
		mustExec(`INSERT INTO messages
			(session_id, ordinal, role, content, timestamp,
			 model, token_usage,
			 claude_message_id, claude_request_id,
			 has_output_tokens)
			VALUES ('s1', ?, 'assistant', '', '2026-04-10T10:02:00Z',
			        'claude-opus-4-6', ?, '', '', 1)`, ord, usage)
	}

	result, err := d.GetDailyUsage(context.Background(), UsageFilter{
		From: "2026-04-10", To: "2026-04-10", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("GetDailyUsage: %v", err)
	}
	if len(result.Daily) != 1 || result.Daily[0].OutputTokens != 20 {
		t.Errorf("output = %v, want 20 (both no-key rows counted)",
			result.Daily)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:
`go test -tags fts5 ./internal/db/ -run 'TestGetDailyUsage_Dedupes|TestGetDailyUsage_MissingDedupKeys' -v`
Expected: FAIL — shared message is counted twice (input=220, output=1080, etc.).

- [ ] **Step 3: Update `GetDailyUsage` to select and dedup**

Edit `internal/db/usage.go:139-249`. Replace the query string and the
scan/accumulator loop. The key changes:

1. Add `m.claude_message_id` and `m.claude_request_id` to the SELECT list.
1. Add
   `ORDER BY COALESCE(m.timestamp, s.started_at) ASC, m.session_id ASC, m.ordinal ASC`
   to make first-wins deterministic.
1. Declare `type dedupKey struct{ msgID, reqID string }` +
   `seen := make(map[dedupKey]struct{})` alongside `accum`.
1. After scanning each row, if both IDs are non-empty and the pair is in `seen`,
   `continue`. Otherwise record it and fall through.

The full replacement block (from line 139 through the end of the row loop at
line 244):

```go
	query := `
SELECT
	COALESCE(m.timestamp, s.started_at) as ts,
	m.model,
	m.token_usage,
	m.claude_message_id,
	m.claude_request_id
FROM messages m
JOIN sessions s ON m.session_id = s.id
WHERE m.token_usage != ''
	AND m.model != ''
	AND m.model != '<synthetic>'
	AND s.deleted_at IS NULL`

	var args []any

	// Filter on message timestamp (not session started_at) so
	// long-lived sessions that span date boundaries are included.
	// Pad by ±14h to cover all timezone offsets — the actual
	// date filtering happens post-query via localDate.
	if f.From != "" {
		padded := paddedUTCBound(f.From+"T00:00:00Z", -14)
		query += " AND COALESCE(m.timestamp, s.started_at) >= ?"
		args = append(args, padded)
	}
	if f.To != "" {
		padded := paddedUTCBound(f.To+"T23:59:59Z", 14)
		query += " AND COALESCE(m.timestamp, s.started_at) <= ?"
		args = append(args, padded)
	}
	if f.Agent != "" {
		query += " AND s.agent = ?"
		args = append(args, f.Agent)
	}

	// Deterministic order so dedup "first row wins" is stable
	// across runs — physical storage order changes under VACUUM.
	query += ` ORDER BY COALESCE(m.timestamp, s.started_at) ASC,
		m.session_id ASC, m.ordinal ASC`

	rows, err := db.getReader().QueryContext(ctx, query, args...)
	if err != nil {
		return DailyUsageResult{},
			fmt.Errorf("querying daily usage: %w", err)
	}
	defer rows.Close()

	// dateModel key for per-(date, model) accumulation
	type dateModelKey struct {
		date  string
		model string
	}
	type modelAccum struct {
		inputTok  int
		outputTok int
		cacheCr   int
		cacheRd   int
		cost      float64
	}

	accum := make(map[dateModelKey]*modelAccum)

	// Global dedup by Claude's (message.id, requestId) — matches
	// ccusage's `${messageId}:${requestId}` strategy. Rows with
	// either key empty (legacy data, non-Claude agents) are
	// always counted, matching ccusage's null-hash fallback in
	// data-loader.ts createUniqueHash. Struct key avoids an
	// alloc per row and can't collide on literal colons in IDs.
	type dedupKey struct {
		msgID, reqID string
	}
	seen := make(map[dedupKey]struct{})

	var (
		ts         string
		model      string
		tokenJSON  string
		msgID      string
		reqID      string
	)
	for rows.Next() {
		if err := rows.Scan(&ts, &model, &tokenJSON, &msgID, &reqID); err != nil {
			return DailyUsageResult{},
				fmt.Errorf("scanning daily usage row: %w", err)
		}

		if msgID != "" && reqID != "" {
			key := dedupKey{msgID: msgID, reqID: reqID}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
		}

		date := localDate(ts, loc)
		if f.From != "" && date < f.From {
			continue
		}
		if f.To != "" && date > f.To {
			continue
		}

		// token_usage is written by our parsers and never by
		// user input, so we trust it to be valid JSON.
		usage := gjson.Parse(tokenJSON)
		inputTok := int(usage.Get("input_tokens").Int())
		outputTok := int(usage.Get("output_tokens").Int())
		cacheCrTok := int(usage.Get("cache_creation_input_tokens").Int())
		cacheRdTok := int(usage.Get("cache_read_input_tokens").Int())

		rates := pricing[model]
		cost := (float64(inputTok)*rates.input +
			float64(outputTok)*rates.output +
			float64(cacheCrTok)*rates.cacheCreation +
			float64(cacheRdTok)*rates.cacheRead) / 1_000_000

		key := dateModelKey{date: date, model: model}
		ma, ok := accum[key]
		if !ok {
			ma = &modelAccum{}
			accum[key] = ma
		}
		ma.inputTok += inputTok
		ma.outputTok += outputTok
		ma.cacheCr += cacheCrTok
		ma.cacheRd += cacheRdTok
		ma.cost += cost
	}
	if err := rows.Err(); err != nil {
		return DailyUsageResult{},
			fmt.Errorf("iterating daily usage rows: %w", err)
	}
```

- [ ] **Step 4: Run the dedup tests to verify they pass**

Run:
`go test -tags fts5 ./internal/db/ -run 'TestGetDailyUsage_Dedupes|TestGetDailyUsage_MissingDedupKeys' -v`
Expected: PASS.

- [ ] **Step 5: Run the full db test suite**

Run: `go test -tags fts5 ./internal/db/ -v` Expected: all PASS. If an existing
`TestGetDailyUsage*` test fails, verify whether its fixture now triggers dedup —
if the test was exercising duplicate rows that should be deduped, update its
expectation. Otherwise the regression is a bug; fix it.

- [ ] **Step 6: Commit**

```bash
git add internal/db/usage.go internal/db/usage_test.go
git commit -m "fix(usage): dedup by Claude message.id+requestId in GetDailyUsage

Aligns agentsview's token sums with ccusage by skipping rows that
share a \`\${message.id}:\${requestId}\` hash already seen during
the scan. Rows with either key empty (legacy data, non-Claude
agents) are always counted, matching ccusage's null-hash
fallback. Fixes ~3x over-count of Claude output tokens caused by
DAG-fork sessions storing duplicate copies of the same API
response."
```

______________________________________________________________________

## Task 8: Forward-compat orphaned session copy

**Why:** When a future `dataVersion` bump triggers a resync,
`CopyOrphanedDataFrom` (`internal/db/orphaned.go:100-140`) copies message rows
from the old DB to the new DB for sessions whose source files are gone. It uses
a column-probe loop to only copy columns present in the old schema. Today's 8→9
bump won't benefit orphaned rows (the old DB has no dedup keys to copy), but
future bumps need to preserve them. Add the two new columns to the probe list so
they flow forward.

**Files:**

- Modify: `internal/db/orphaned.go:123-131`

- [ ] **Step 1: Read the existing column probe block**

Run: `sed -n '115,135p' internal/db/orphaned.go` Expected: shows the
`msgCols := "session_id, ordinal, ..."` block and the
`for _, c := range []string{...}` loop that conditionally appends columns if the
old DB has them.

- [ ] **Step 2: Add the new columns to the probe list**

Edit `internal/db/orphaned.go` around lines 123-131. Append
`"claude_message_id"` and `"claude_request_id"` to the end of the string slice:

```go
	for _, c := range []string{
		"model", "token_usage", "context_tokens",
		"output_tokens", "has_context_tokens",
		"has_output_tokens",
		"claude_message_id", "claude_request_id",
	} {
		if oldDBHasColumn(ctx, tx, "messages", c) {
			msgCols += ", " + c
		}
	}
```

- [ ] **Step 3: Verify the db package still builds and tests pass**

Run: `go test -tags fts5 ./internal/db/ -v` Expected: all PASS. Existing
orphaned-data tests (if any) should be unaffected since the probe returns false
for the 8→9 bump source schema.

- [ ] **Step 4: Commit**

```bash
git add internal/db/orphaned.go
git commit -m "chore(db): include claude dedup columns in orphaned copy probe

Ensures a future dataVersion bump preserves claude_message_id
and claude_request_id on orphaned messages (sessions whose
source files are gone). The current 8->9 bump can't benefit
these rows — the old DB has no keys to copy — but forward
bumps will. Limitation documented in the spec."
```

______________________________________________________________________

## Task 9: Bump `dataVersion` to trigger resync

**Why:** Existing databases have empty `claude_message_id` / `claude_request_id`
values. Without a full resync, those rows never get deduped and users keep
seeing inflated totals until they rewrite their DB manually. Bumping
`dataVersion` makes `NeedsResync()` return true on next open, which triggers
`ResyncAll` in `cmd/agentsview/main.go:245`, `cmd/agentsview/usage.go:226`, and
`cmd/agentsview/sync.go:111`.

**Files:**

- Modify: `internal/db/db.go:28` — constant value

- [ ] **Step 1: Bump the constant**

Edit `internal/db/db.go` line 28 from `const dataVersion = 8` to:

```go
const dataVersion = 9
```

- [ ] **Step 2: Verify resync-related tests still pass**

Run: `go test -tags fts5 ./internal/db/ -run 'DataVersion|Resync' -v` Expected:
PASS. Tests may hard-code `8` — if so, update them to `9` since the bump is
intentional.

- [ ] **Step 3: Commit**

```bash
git add internal/db/db.go
git commit -m "chore(db): bump dataVersion to 9 for claude dedup-key backfill

Forces existing databases to re-parse Claude session files so
the new claude_message_id / claude_request_id columns get
populated, enabling cross-session token dedup for historical
data on first run."
```

______________________________________________________________________

## Task 10: End-to-end parity check against ccusage

**Why:** Final verification that the fix actually closes the gap on a real DB.
This is a manual verification script — not a committed test — because it depends
on the user's local `~/.claude/projects` data.

**Files:** none modified.

- [ ] **Step 1: Rebuild agentsview with the fix**

Run: `make build` Expected: binary at `./bin/agentsview` (or wherever Makefile
installs it).

- [ ] **Step 2: Run the new binary's daily usage for the last 30 days**

Run:
`./bin/agentsview usage daily --agent claude --json > /tmp/agentsview_fixed.json && jq '.totals' /tmp/agentsview_fixed.json`
Expected: first run prints a "Data version changed, running full resync..."
message on stderr, then emits JSON. The resync will take a while depending on DB
size.

- [ ] **Step 3: Run ccusage for the same window**

Run (adjust `--since/--until` to match agentsview's default 30-day window, which
is `today - 29` through `today`):

```
ccusage daily --json --offline --since $(date -v-29d +%Y%m%d) --until $(date +%Y%m%d) | jq '.totals'
```

- [ ] **Step 4: Compare the two `.totals` outputs**

Expected: `inputTokens`, `outputTokens`, `cacheCreationTokens`,
`cacheReadTokens` match ccusage within ~1% (small drift from timezone bucketing
is acceptable, but the output-token ratio should no longer be ~3x).

- [ ] **Step 5: If totals still don't match**

Do NOT start another fix blindly. Go back to Phase 1 of systematic debugging:

- Pick a single date where the two tools disagree the most.

- Dump `--breakdown` from both for that date.

- Grep `~/.claude/projects` for the specific msg.ids and verify manually.

- The dedup logic in Task 7 is ccusage's exact algorithm, so remaining gaps are
  most likely: (a) timezone bucketing around day boundaries (tolerable); (b)
  model-name matching failing in pricing lookup (cost drift, not tokens); (c)
  messages with missing `message.id`/`requestId` (legitimately counted, just
  accounted for differently).

- [ ] **Step 6: If totals match, push the branch**

Ask the user before pushing. They explicitly own the push decision.

______________________________________________________________________

## Self-Review Notes

- **Spec coverage:** all issues from the investigation are covered — parser
  extraction (Tasks 2-3), schema + write path (Tasks 4-6), query-time dedup
  (Task 7), orphaned-copy forward compat (Task 8), resync trigger (Task 9), plus
  the Opus-4-6 pricing side-quest (Task 1) and end-to-end verification (Task
  10). PG push parity deferred per spec decision.
- **Placeholder scan:** no `TODO`/`TBD`/"similar to Task N" placeholders.
- **Type consistency:** `ClaudeMessageID` and `ClaudeRequestID` used
  consistently on `parser.ParsedMessage` (Task 2), `db.Message` (Task 5), and
  the `toDBMessages` bridge (Task 6). The SQL columns are `claude_message_id` /
  `claude_request_id` throughout (Tasks 4, 5, 7, 8).
- **Known risk — Task 5 Step 3:** updating every caller of `selectMessageCols`
  is the single most error-prone part of this plan. Out-of-sync scanners are a
  silent source of runtime errors. Run
  `rg -n "selectMessageCols|rowsToMessages|scanMessage"` and update every hit
  before moving on.
