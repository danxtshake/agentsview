# Session Activity Minimap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a togglable horizontal stacked bar chart to the session
detail view that shows message activity intensity over time, with
click-to-navigate.

**Architecture:** New `GET /api/v1/sessions/{id}/activity` endpoint
returns time-bucketed message counts (adaptive interval). SQLite and
PostgreSQL each get their own query. Frontend renders a custom SVG
component between `SessionBreadcrumb` and `MessageList`, synced to
scroll position via a shared `firstVisibleTimestamp` value.

**Tech Stack:** Go (SQLite + PostgreSQL), Svelte 5, custom SVG, CSS
variables for theming.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/db/activity.go` | Types + SQLite bucketing query |
| Create | `internal/db/activity_test.go` | SQLite activity tests |
| Create | `internal/postgres/activity.go` | PostgreSQL bucketing query |
| (CI) | `internal/postgres/activity_test.go` | PG tests via `make test-postgres` |
| Modify | `internal/db/store.go` | Add `GetSessionActivity` to interface |
| Modify | `internal/server/server.go` | Register `/activity` route, remove `/minimap` |
| Create | `internal/server/activity.go` | HTTP handler for activity endpoint |
| Create | `internal/server/activity_test.go` | Handler tests |
| Modify | `internal/db/messages.go` | Remove `MinimapEntry`, `GetMinimap*`, `SampleMinimap` |
| Modify | `internal/db/db_test.go` | Remove `TestSampleMinimap` |
| Modify | `internal/server/messages.go` | Remove `handleGetMinimap` |
| Modify | `internal/server/server_test.go` | Remove minimap test cases |
| Modify | `internal/server/deadline_test.go` | Remove minimap deadline test |
| Modify | `internal/server/deadline_internal_test.go` | Remove minimap internal test |
| Modify | `internal/postgres/messages.go` | Remove `GetMinimap`, `GetMinimapFrom` |
| Modify | `internal/postgres/store_test.go` | Remove minimap mock references (if any) |
| Create | `frontend/src/lib/api/types/session-activity.ts` | `SessionActivityBucket`, `SessionActivityResponse` |
| Modify | `frontend/src/lib/api/client.ts` | Add `getSessionActivity`, remove `getMinimap` |
| Modify | `frontend/src/lib/api/types/core.ts` | Remove `MinimapEntry`, `MinimapResponse` |
| Create | `frontend/src/lib/stores/sessionActivity.svelte.ts` | Activity minimap store |
| Create | `frontend/src/lib/stores/sessionActivity.test.ts` | Store unit tests |
| Modify | `frontend/src/lib/stores/ui.svelte.ts` | Add `activityMinimapOpen` state |
| Modify | `frontend/src/lib/components/content/MessageList.svelte` | Publish `firstVisibleTimestamp` |
| Create | `frontend/src/lib/components/content/ActivityMinimap.svelte` | SVG bar chart component |
| Modify | `frontend/src/lib/components/layout/SessionBreadcrumb.svelte` | Toggle button |
| Modify | `frontend/src/App.svelte` | Insert `ActivityMinimap` between breadcrumb and list |

---

## Task 1: Remove dead minimap code (backend)

**Files:**
- Modify: `internal/db/messages.go:76-207`
- Modify: `internal/db/store.go:31-32`
- Modify: `internal/db/db_test.go` (TestSampleMinimap)
- Modify: `internal/server/messages.go:51-95`
- Modify: `internal/server/server.go:168` (route)
- Modify: `internal/server/server_test.go` (TestGetMinimap*)
- Modify: `internal/server/deadline_test.go:24`
- Modify: `internal/server/deadline_internal_test.go:35`
- Modify: `internal/postgres/messages.go:88-125`

- [ ] **Step 1: Remove `MinimapEntry` type, `GetMinimap`, `GetMinimapFrom`, `SampleMinimap` from `internal/db/messages.go`**

Delete lines 76-207 (the `MinimapEntry` struct, `GetMinimap`,
`GetMinimapFrom`, and `SampleMinimap` functions).

- [ ] **Step 2: Remove `GetMinimap` and `GetMinimapFrom` from Store interface in `internal/db/store.go`**

Delete lines 31-32:
```go
	GetMinimap(ctx context.Context, sessionID string) ([]MinimapEntry, error)
	GetMinimapFrom(ctx context.Context, sessionID string, from int) ([]MinimapEntry, error)
```

- [ ] **Step 3: Remove `handleGetMinimap` from `internal/server/messages.go`**

Delete the entire `handleGetMinimap` function (lines 51-95). Also remove
the `dbpkg` import alias if it was only used by `SampleMinimap`:

```go
// Remove this import if no other reference exists:
dbpkg "github.com/wesm/agentsview/internal/db"
```

Check remaining references first — `handleGetMessages` may also use
`dbpkg`.

- [ ] **Step 4: Remove minimap route from `internal/server/server.go`**

Delete the route registration:
```go
	s.mux.Handle(
		"GET /api/v1/sessions/{id}/minimap", s.withTimeout(s.handleGetMinimap),
	)
```

- [ ] **Step 5: Remove `GetMinimap` and `GetMinimapFrom` from `internal/postgres/messages.go`**

Delete the two methods (lines ~88-125).

- [ ] **Step 6: Remove minimap test cases from backend tests**

In `internal/server/server_test.go`, remove:
- The `minimapResponse` struct definition (lines ~570-572)
- `TestGetMinimap` (lines ~951-986)
- `TestGetMinimap_FromOrdinal` (lines ~988-1004)
- `TestGetMinimap_InvalidFrom` (lines ~1006-1013)
- `TestGetMinimap_MaxSampled` (lines ~1015-1037)
- `TestGetMinimap_InvalidMax` (lines ~1039-1046)

In `internal/db/db_test.go`, remove `TestSampleMinimap` (lines
~2144-2230).

In `internal/server/deadline_test.go`, remove the minimap entry from the
test table:
```go
{"GetMinimap", http.MethodGet, "/api/v1/sessions/s1/minimap"},
```

In `internal/server/deadline_internal_test.go`, remove the minimap entry
from the handler table:
```go
{"GetMinimap", s.handleGetMinimap, false},
```

- [ ] **Step 7: Grep for remaining minimap references and remove**

```bash
rg -i minimap --type go
```

Remove any remaining references not caught above (e.g. in
`internal/postgres/store_test.go` if present).

- [ ] **Step 8: Verify backend compiles and tests pass**

```bash
CGO_ENABLED=1 go build -tags fts5 ./...
CGO_ENABLED=1 go test -tags fts5 ./internal/db/... ./internal/server/... -count=1
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: remove unused minimap endpoint and supporting code"
```

---

## Task 2: Remove dead minimap code (frontend)

**Files:**
- Modify: `frontend/src/lib/api/client.ts:193-205`
- Modify: `frontend/src/lib/api/types/core.ts:75-110`

- [ ] **Step 1: Remove `MinimapEntry` and `MinimapResponse` from `frontend/src/lib/api/types/core.ts`**

Delete the `MinimapEntry` type alias and `MinimapResponse` interface
(lines ~75-110).

- [ ] **Step 2: Remove `getMinimap` and `GetMinimapParams` from `frontend/src/lib/api/client.ts`**

Delete the `GetMinimapParams` interface and `getMinimap` function (lines
~193-205). Remove the `MinimapResponse` import if it becomes unused.

- [ ] **Step 3: Grep for remaining frontend minimap references**

```bash
rg -i minimap frontend/src/
```

Remove any remaining imports or references (check
`frontend/src/lib/stores/messages.test.ts` and any re-export barrels).

- [ ] **Step 4: Verify frontend compiles**

```bash
cd frontend && npx tsc --noEmit && cd ..
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove unused minimap types and client method from frontend"
```

---

## Task 3: Add `SessionActivityBucket` type and `GetSessionActivity` to Store interface

**Files:**
- Create: `internal/db/activity.go`
- Modify: `internal/db/store.go`

- [ ] **Step 1: Write the failing test for interval snapping**

Create `internal/db/activity_test.go`:

```go
package db

import "testing"

func TestSnapInterval(t *testing.T) {
	tests := []struct {
		name     string
		duration int64 // seconds
		want     int64
	}{
		{"30s session", 30, 60},
		{"5m session", 300, 60},
		{"10m session", 600, 60},
		{"20m session", 1200, 60},
		{"30m session", 1800, 120},
		{"1h session", 3600, 120},
		{"2h session", 7200, 300},
		{"4h session", 14400, 600},
		{"8h session", 28800, 900},
		{"12h session", 43200, 1800},
		{"16h session", 57600, 3600},
		{"24h session", 86400, 3600},
		{"48h session", 172800, 7200},
		{"0s session", 0, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapInterval(tt.duration)
			if got != tt.want {
				t.Errorf(
					"snapInterval(%d) = %d, want %d",
					tt.duration, got, tt.want,
				)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test -tags fts5 -run TestSnapInterval ./internal/db/ -v
```

Expected: FAIL — `snapInterval` undefined.

- [ ] **Step 3: Create `internal/db/activity.go` with types and `snapInterval`**

```go
package db

import "context"

// SessionActivityBucket holds message counts for one time interval.
type SessionActivityBucket struct {
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	UserCount    int    `json:"user_count"`
	AssistantCount int  `json:"assistant_count"`
	FirstOrdinal *int   `json:"first_ordinal"` // nil for empty buckets
}

// SessionActivityResponse is the response for the activity endpoint.
type SessionActivityResponse struct {
	Buckets         []SessionActivityBucket `json:"buckets"`
	IntervalSeconds int64                   `json:"interval_seconds"`
	TotalMessages   int                     `json:"total_messages"`
}

// intervalSteps are the allowed bucket widths in seconds.
// [1m, 2m, 5m, 10m, 15m, 30m, 1h, 2h]
var intervalSteps = []int64{
	60, 120, 300, 600, 900, 1800, 3600, 7200,
}

// snapInterval picks the interval step that produces ~20-40 buckets
// for a session of the given duration (in seconds).
func snapInterval(durationSec int64) int64 {
	if durationSec <= 0 {
		return intervalSteps[0]
	}
	target := durationSec / 30
	if target <= 0 {
		return intervalSteps[0]
	}
	best := intervalSteps[0]
	for _, step := range intervalSteps {
		best = step
		if step >= target {
			break
		}
	}
	return best
}

// GetSessionActivity returns time-bucketed message counts for a
// session. Only visible messages are counted (system and
// prefix-detected injected messages excluded).
func (d *DB) GetSessionActivity(
	ctx context.Context, sessionID string,
) (*SessionActivityResponse, error) {
	return getSessionActivitySQLite(d, ctx, sessionID)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
CGO_ENABLED=1 go test -tags fts5 -run TestSnapInterval ./internal/db/ -v
```

Expected: PASS.

- [ ] **Step 5: Add `GetSessionActivity` to the Store interface in `internal/db/store.go`**

Add under the `// Messages.` section, replacing the removed minimap
methods:

```go
	// Messages.
	GetMessages(ctx context.Context, sessionID string, from, limit int, asc bool) ([]Message, error)
	GetAllMessages(ctx context.Context, sessionID string) ([]Message, error)
	GetSessionActivity(ctx context.Context, sessionID string) (*SessionActivityResponse, error)
```

- [ ] **Step 6: Verify it compiles (expect PG store failure)**

```bash
CGO_ENABLED=1 go build -tags fts5 ./internal/db/...
```

Expected: compiles. The PG store will fail separately — that's expected
and fixed in Task 5.

- [ ] **Step 7: Commit**

```bash
git add internal/db/activity.go internal/db/activity_test.go internal/db/store.go
git commit -m "feat: add SessionActivity types, interval snapping, and Store interface method"
```

---

## Task 4: Implement SQLite activity bucketing query

**Files:**
- Modify: `internal/db/activity.go`
- Modify: `internal/db/activity_test.go`

- [ ] **Step 1: Write the failing test for SQLite bucketing**

Append to `internal/db/activity_test.go`:

```go
func TestGetSessionActivity(t *testing.T) {
	d := testDB(t)
	sid := "test-activity"

	// Insert a session.
	err := d.UpsertSession(Session{
		ID:        sid,
		Agent:     "claude",
		StartedAt: "2026-03-26T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Insert messages spanning 30 minutes.
	msgs := []Message{
		{SessionID: sid, Ordinal: 0, Role: "user", Content: "hello", Timestamp: "2026-03-26T10:00:00Z", ContentLength: 5},
		{SessionID: sid, Ordinal: 1, Role: "assistant", Content: "hi", Timestamp: "2026-03-26T10:00:30Z", ContentLength: 2},
		{SessionID: sid, Ordinal: 2, Role: "user", Content: "next", Timestamp: "2026-03-26T10:01:30Z", ContentLength: 4},
		{SessionID: sid, Ordinal: 3, Role: "assistant", Content: "resp", Timestamp: "2026-03-26T10:02:00Z", ContentLength: 4},
		// Gap: no messages from 10:02 to 10:28.
		{SessionID: sid, Ordinal: 4, Role: "user", Content: "back", Timestamp: "2026-03-26T10:28:00Z", ContentLength: 4},
		{SessionID: sid, Ordinal: 5, Role: "assistant", Content: "wb", Timestamp: "2026-03-26T10:29:00Z", ContentLength: 2},
		// System message — should be excluded.
		{SessionID: sid, Ordinal: 6, Role: "user", Content: "This session is being continued from a previous conversation.", Timestamp: "2026-03-26T10:29:30Z", ContentLength: 60, IsSystem: true},
	}
	if err := d.InsertMessages(msgs); err != nil {
		t.Fatal(err)
	}

	resp, err := d.GetSessionActivity(
		context.Background(), sid,
	)
	if err != nil {
		t.Fatal(err)
	}

	// 29 min span => 1min buckets (snapInterval(1740) = 60).
	if resp.IntervalSeconds != 60 {
		t.Errorf(
			"interval = %d, want 60",
			resp.IntervalSeconds,
		)
	}

	// System message excluded from buckets.
	if resp.TotalMessages != 7 {
		t.Errorf(
			"total = %d, want 7",
			resp.TotalMessages,
		)
	}

	// Should have 30 buckets (min 0 to min 29).
	if len(resp.Buckets) < 28 {
		t.Errorf(
			"bucket count = %d, want >= 28",
			len(resp.Buckets),
		)
	}

	// First bucket should have user=1, assistant=1.
	first := resp.Buckets[0]
	if first.UserCount != 1 || first.AssistantCount != 1 {
		t.Errorf(
			"first bucket: user=%d asst=%d, want 1,1",
			first.UserCount, first.AssistantCount,
		)
	}
	if first.FirstOrdinal == nil || *first.FirstOrdinal != 0 {
		t.Errorf("first bucket first_ordinal: got %v, want 0", first.FirstOrdinal)
	}

	// Middle empty buckets should have nil FirstOrdinal.
	mid := resp.Buckets[15]
	if mid.UserCount != 0 || mid.AssistantCount != 0 {
		t.Errorf(
			"mid bucket: user=%d asst=%d, want 0,0",
			mid.UserCount, mid.AssistantCount,
		)
	}
	if mid.FirstOrdinal != nil {
		t.Errorf("mid bucket first_ordinal: got %v, want nil", mid.FirstOrdinal)
	}
}

func TestGetSessionActivity_NoMessages(t *testing.T) {
	d := testDB(t)
	sid := "test-empty"

	err := d.UpsertSession(Session{
		ID:    sid,
		Agent: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := d.GetSessionActivity(
		context.Background(), sid,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Buckets) != 0 {
		t.Errorf("buckets = %d, want 0", len(resp.Buckets))
	}
}

func TestGetSessionActivity_NullTimestamps(t *testing.T) {
	d := testDB(t)
	sid := "test-null-ts"

	err := d.UpsertSession(Session{
		ID: sid, Agent: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []Message{
		{SessionID: sid, Ordinal: 0, Role: "user", Content: "hi", ContentLength: 2},
		{SessionID: sid, Ordinal: 1, Role: "assistant", Content: "hello", ContentLength: 5},
	}
	if err := d.InsertMessages(msgs); err != nil {
		t.Fatal(err)
	}

	resp, err := d.GetSessionActivity(
		context.Background(), sid,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Buckets) != 0 {
		t.Errorf("buckets = %d, want 0", len(resp.Buckets))
	}
	if resp.TotalMessages != 2 {
		t.Errorf("total = %d, want 2", resp.TotalMessages)
	}
}

func TestGetSessionActivity_SingleMessage(t *testing.T) {
	d := testDB(t)
	sid := "test-single"

	err := d.UpsertSession(Session{
		ID: sid, Agent: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}

	msgs := []Message{
		{SessionID: sid, Ordinal: 0, Role: "user", Content: "hi", Timestamp: "2026-03-26T10:00:00Z", ContentLength: 2},
	}
	if err := d.InsertMessages(msgs); err != nil {
		t.Fatal(err)
	}

	resp, err := d.GetSessionActivity(
		context.Background(), sid,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Buckets) != 1 {
		t.Fatalf("buckets = %d, want 1", len(resp.Buckets))
	}
	if resp.Buckets[0].UserCount != 1 {
		t.Errorf("user count = %d, want 1", resp.Buckets[0].UserCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test -tags fts5 -run TestGetSessionActivity ./internal/db/ -v
```

Expected: FAIL — `getSessionActivitySQLite` undefined.

- [ ] **Step 3: Implement `getSessionActivitySQLite` in `internal/db/activity.go`**

Add to `internal/db/activity.go`:

```go
import (
	"context"
	"fmt"
	"time"
)

func getSessionActivitySQLite(
	d *DB, ctx context.Context, sessionID string,
) (*SessionActivityResponse, error) {
	// Count total messages (including system, for the total field).
	var totalMessages int
	err := d.getReader().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?",
		sessionID,
	).Scan(&totalMessages)
	if err != nil {
		return nil, fmt.Errorf("counting messages: %w", err)
	}

	// Visible message filter: exclude is_system and prefix-detected.
	visFilter := "m.is_system = 0 AND " +
		SystemPrefixSQL("m.content", "m.role")

	// Get min/max timestamps from visible messages.
	var minTS, maxTS *string
	err = d.getReader().QueryRowContext(ctx, `
		SELECT MIN(m.timestamp), MAX(m.timestamp)
		FROM messages m
		WHERE m.session_id = ?
			AND m.timestamp IS NOT NULL
			AND m.timestamp != ''
			AND `+visFilter,
		sessionID,
	).Scan(&minTS, &maxTS)
	if err != nil {
		return nil, fmt.Errorf("getting timestamp range: %w", err)
	}

	if minTS == nil || maxTS == nil {
		return &SessionActivityResponse{
			Buckets:       []SessionActivityBucket{},
			TotalMessages: totalMessages,
		}, nil
	}

	tMin, err := time.Parse(time.RFC3339, *minTS)
	if err != nil {
		return &SessionActivityResponse{
			Buckets:       []SessionActivityBucket{},
			TotalMessages: totalMessages,
		}, nil
	}
	tMax, err := time.Parse(time.RFC3339, *maxTS)
	if err != nil {
		return &SessionActivityResponse{
			Buckets:       []SessionActivityBucket{},
			TotalMessages: totalMessages,
		}, nil
	}

	durationSec := int64(tMax.Sub(tMin).Seconds())
	interval := snapInterval(durationSec)
	epochMin := tMin.Unix()

	// Query: bucket visible messages by interval.
	rows, err := d.getReader().QueryContext(ctx, `
		SELECT
			(CAST(strftime('%s', m.timestamp) AS INTEGER) - ?) / ? AS bucket,
			SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END),
			SUM(CASE WHEN m.role = 'assistant' THEN 1 ELSE 0 END),
			MIN(m.ordinal)
		FROM messages m
		WHERE m.session_id = ?
			AND m.timestamp IS NOT NULL
			AND m.timestamp != ''
			AND `+visFilter+`
		GROUP BY bucket
		ORDER BY bucket ASC`,
		epochMin, interval, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("bucketing activity: %w", err)
	}
	defer rows.Close()

	// Collect populated buckets.
	type rawBucket struct {
		idx      int64
		userCt   int
		asstCt   int
		firstOrd int
	}
	populated := map[int64]rawBucket{}
	var maxIdx int64
	for rows.Next() {
		var rb rawBucket
		if err := rows.Scan(
			&rb.idx, &rb.userCt, &rb.asstCt, &rb.firstOrd,
		); err != nil {
			return nil, fmt.Errorf("scanning bucket: %w", err)
		}
		populated[rb.idx] = rb
		if rb.idx > maxIdx {
			maxIdx = rb.idx
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(populated) == 0 {
		return &SessionActivityResponse{
			Buckets:         []SessionActivityBucket{},
			IntervalSeconds: interval,
			TotalMessages:   totalMessages,
		}, nil
	}

	// Build full bucket array including empty gaps.
	buckets := make(
		[]SessionActivityBucket, 0, maxIdx+1,
	)
	for i := int64(0); i <= maxIdx; i++ {
		start := time.Unix(epochMin+i*interval, 0).UTC()
		end := time.Unix(
			epochMin+(i+1)*interval, 0,
		).UTC()
		b := SessionActivityBucket{
			StartTime: start.Format(time.RFC3339),
			EndTime:   end.Format(time.RFC3339),
		}
		if rb, ok := populated[i]; ok {
			b.UserCount = rb.userCt
			b.AssistantCount = rb.asstCt
			b.FirstOrdinal = &rb.firstOrd
		}
		buckets = append(buckets, b)
	}

	return &SessionActivityResponse{
		Buckets:         buckets,
		IntervalSeconds: interval,
		TotalMessages:   totalMessages,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test -tags fts5 -run TestGetSessionActivity ./internal/db/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/activity.go internal/db/activity_test.go
git commit -m "feat: implement SQLite session activity bucketing query"
```

---

## Task 5: Implement PostgreSQL activity bucketing query

**Files:**
- Create: `internal/postgres/activity.go`

- [ ] **Step 1: Create `internal/postgres/activity.go`**

```go
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

// GetSessionActivity returns time-bucketed message counts for a
// session, using PostgreSQL-specific timestamp functions.
func (s *Store) GetSessionActivity(
	ctx context.Context, sessionID string,
) (*db.SessionActivityResponse, error) {
	// Count total messages.
	var totalMessages int
	err := s.pg.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = $1",
		sessionID,
	).Scan(&totalMessages)
	if err != nil {
		return nil, fmt.Errorf("counting messages: %w", err)
	}

	visFilter := "m.is_system = FALSE AND " +
		db.SystemPrefixSQL("m.content", "m.role")

	// Get min/max timestamps from visible messages.
	var minTS, maxTS *time.Time
	err = s.pg.QueryRowContext(ctx, `
		SELECT MIN(m.timestamp), MAX(m.timestamp)
		FROM messages m
		WHERE m.session_id = $1
			AND m.timestamp IS NOT NULL
			AND `+visFilter,
		sessionID,
	).Scan(&minTS, &maxTS)
	if err != nil {
		return nil, fmt.Errorf("getting timestamp range: %w", err)
	}

	if minTS == nil || maxTS == nil {
		return &db.SessionActivityResponse{
			Buckets:       []db.SessionActivityBucket{},
			TotalMessages: totalMessages,
		}, nil
	}

	durationSec := int64(maxTS.Sub(*minTS).Seconds())
	interval := db.SnapInterval(durationSec)
	epochMin := minTS.Unix()

	rows, err := s.pg.QueryContext(ctx, `
		SELECT
			(EXTRACT(EPOCH FROM m.timestamp)::bigint - $1) / $2
				AS bucket,
			SUM(CASE WHEN m.role = 'user'
				THEN 1 ELSE 0 END)::int,
			SUM(CASE WHEN m.role = 'assistant'
				THEN 1 ELSE 0 END)::int,
			MIN(m.ordinal)
		FROM messages m
		WHERE m.session_id = $3
			AND m.timestamp IS NOT NULL
			AND `+visFilter+`
		GROUP BY bucket
		ORDER BY bucket ASC`,
		epochMin, interval, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("bucketing activity: %w", err)
	}
	defer rows.Close()

	type rawBucket struct {
		idx      int64
		userCt   int
		asstCt   int
		firstOrd int
	}
	populated := map[int64]rawBucket{}
	var maxIdx int64
	for rows.Next() {
		var rb rawBucket
		if err := rows.Scan(
			&rb.idx, &rb.userCt, &rb.asstCt, &rb.firstOrd,
		); err != nil {
			return nil, fmt.Errorf("scanning bucket: %w", err)
		}
		populated[rb.idx] = rb
		if rb.idx > maxIdx {
			maxIdx = rb.idx
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(populated) == 0 {
		return &db.SessionActivityResponse{
			Buckets:         []db.SessionActivityBucket{},
			IntervalSeconds: interval,
			TotalMessages:   totalMessages,
		}, nil
	}

	buckets := make(
		[]db.SessionActivityBucket, 0, maxIdx+1,
	)
	for i := int64(0); i <= maxIdx; i++ {
		start := time.Unix(epochMin+i*interval, 0).UTC()
		end := time.Unix(
			epochMin+(i+1)*interval, 0,
		).UTC()
		b := db.SessionActivityBucket{
			StartTime: start.Format(time.RFC3339),
			EndTime:   end.Format(time.RFC3339),
		}
		if rb, ok := populated[i]; ok {
			b.UserCount = rb.userCt
			b.AssistantCount = rb.asstCt
			b.FirstOrdinal = &rb.firstOrd
		}
		buckets = append(buckets, b)
	}

	return &db.SessionActivityResponse{
		Buckets:         buckets,
		IntervalSeconds: interval,
		TotalMessages:   totalMessages,
	}, nil
}
```

**Note:** This references `db.SnapInterval` (exported). You must rename
`snapInterval` to `SnapInterval` in `internal/db/activity.go` so the
PG package can call it. Update the test to use `SnapInterval` too.

- [ ] **Step 2: Export `SnapInterval` in `internal/db/activity.go`**

Rename `snapInterval` to `SnapInterval` and update all references
(including `activity_test.go` test cases and the
`getSessionActivitySQLite` call).

- [ ] **Step 3: Verify full build compiles**

```bash
CGO_ENABLED=1 go build -tags fts5 ./...
```

Expected: compiles (PG store now satisfies the interface).

- [ ] **Step 4: Run all backend tests**

```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/db/... ./internal/server/... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/postgres/activity.go internal/db/activity.go internal/db/activity_test.go
git commit -m "feat: implement PostgreSQL session activity bucketing query"
```

---

## Task 6: Add HTTP handler and route for activity endpoint

**Files:**
- Create: `internal/server/activity.go`
- Create: `internal/server/activity_test.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/activity_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wesm/agentsview/internal/db"
)

func TestGetSessionActivity(t *testing.T) {
	srv, cleanup := testServer(t)
	defer cleanup()

	resp := doGet(t, srv, "/api/v1/sessions/s1/activity")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body db.SessionActivityResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.TotalMessages == 0 {
		t.Error("expected non-zero total_messages")
	}
	if len(body.Buckets) == 0 {
		t.Error("expected non-empty buckets")
	}
	if body.IntervalSeconds <= 0 {
		t.Error("expected positive interval_seconds")
	}
}

func TestGetSessionActivity_NotFound(t *testing.T) {
	srv, cleanup := testServer(t)
	defer cleanup()

	resp := doGet(
		t, srv, "/api/v1/sessions/nonexistent/activity",
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body db.SessionActivityResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Buckets) != 0 {
		t.Error("expected empty buckets for nonexistent session")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test -tags fts5 -run TestGetSessionActivity ./internal/server/ -v
```

Expected: FAIL — 404 (route not registered).

- [ ] **Step 3: Create `internal/server/activity.go`**

```go
package server

import "net/http"

// handleGetSessionActivity returns time-bucketed message counts.
func (s *Server) handleGetSessionActivity(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")

	resp, err := s.db.GetSessionActivity(
		r.Context(), sessionID,
	)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(
			w, http.StatusInternalServerError, err.Error(),
		)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
```

- [ ] **Step 4: Register the route in `internal/server/server.go`**

Add after the messages route:

```go
	s.mux.Handle(
		"GET /api/v1/sessions/{id}/activity",
		s.withTimeout(s.handleGetSessionActivity),
	)
```

- [ ] **Step 5: Run test to verify it passes**

```bash
CGO_ENABLED=1 go test -tags fts5 -run TestGetSessionActivity ./internal/server/ -v
```

Expected: PASS.

- [ ] **Step 6: Add deadline test entry**

In `internal/server/deadline_test.go`, add to the test table:
```go
{"GetSessionActivity", http.MethodGet, "/api/v1/sessions/s1/activity"},
```

In `internal/server/deadline_internal_test.go`, add to the handler
table:
```go
{"GetSessionActivity", s.handleGetSessionActivity, false},
```

- [ ] **Step 7: Run full server tests**

```bash
CGO_ENABLED=1 go test -tags fts5 ./internal/server/... -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/server/activity.go internal/server/activity_test.go internal/server/server.go internal/server/deadline_test.go internal/server/deadline_internal_test.go
git commit -m "feat: add /api/v1/sessions/{id}/activity HTTP endpoint"
```

---

## Task 7: Add frontend types and API client method

**Files:**
- Create: `frontend/src/lib/api/types/session-activity.ts`
- Modify: `frontend/src/lib/api/client.ts`

- [ ] **Step 1: Create `frontend/src/lib/api/types/session-activity.ts`**

```typescript
export interface SessionActivityBucket {
  start_time: string;
  end_time: string;
  user_count: number;
  assistant_count: number;
  first_ordinal: number | null;
}

export interface SessionActivityResponse {
  buckets: SessionActivityBucket[];
  interval_seconds: number;
  total_messages: number;
}
```

- [ ] **Step 2: Add the re-export to the types barrel (if one exists)**

Check if `frontend/src/lib/api/types.ts` or similar re-exports types.
If so, add:

```typescript
export type {
  SessionActivityBucket,
  SessionActivityResponse,
} from "./types/session-activity.js";
```

- [ ] **Step 3: Add `getSessionActivity` to `frontend/src/lib/api/client.ts`**

```typescript
import type { SessionActivityResponse } from "./types/session-activity.js";

export function getSessionActivity(
  sessionId: string,
): Promise<SessionActivityResponse> {
  return fetchJSON(
    `/sessions/${sessionId}/activity`,
  );
}
```

- [ ] **Step 4: Verify frontend compiles**

```bash
cd frontend && npx tsc --noEmit && cd ..
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/api/types/session-activity.ts frontend/src/lib/api/client.ts
git commit -m "feat: add session activity types and API client method"
```

---

## Task 8: Add `activityMinimapOpen` to UI store and `firstVisibleTimestamp` to MessageList

**Files:**
- Modify: `frontend/src/lib/stores/ui.svelte.ts`
- Modify: `frontend/src/lib/components/content/MessageList.svelte`

- [ ] **Step 1: Add `activityMinimapOpen` to UI store**

In `frontend/src/lib/stores/ui.svelte.ts`, add to the `UIStore` class:

```typescript
const MINIMAP_KEY = "agentsview-activity-minimap";
```

Add at top of file alongside other storage key constants. Then in the
class:

```typescript
  activityMinimapOpen: boolean = $state(
    readStoredBool(MINIMAP_KEY, false),
  );
```

Add a persistence effect in the constructor `$effect.root` block:

```typescript
      $effect(() => {
        try {
          localStorage?.setItem(
            MINIMAP_KEY,
            String(this.activityMinimapOpen),
          );
        } catch {
          // ignore
        }
      });
```

Add the `readStoredBool` helper at file scope:

```typescript
function readStoredBool(key: string, fallback: boolean): boolean {
  try {
    const raw = localStorage?.getItem(key);
    if (raw === "true") return true;
    if (raw === "false") return false;
  } catch {
    // ignore
  }
  return fallback;
}
```

Add a toggle method:

```typescript
  toggleActivityMinimap() {
    this.activityMinimapOpen = !this.activityMinimapOpen;
  }
```

- [ ] **Step 2: Publish `firstVisibleTimestamp` from MessageList**

In `frontend/src/lib/components/content/MessageList.svelte`, add a new
exported getter alongside the existing exports (after line ~273):

```typescript
  export function getFirstVisibleTimestamp(): string | null {
    const items = virtualizer.instance?.getVirtualItems() ?? [];
    for (const vi of items) {
      const item = displayItemsAsc[
        ui.sortNewestFirst
          ? displayItemsAsc.length - 1 - vi.index
          : vi.index
      ];
      if (!item) continue;
      const ts =
        item.kind === "message"
          ? item.message.timestamp
          : item.timestamp;
      if (ts) return ts;
    }
    return null;
  }
```

Also update the `handleScroll` function to publish to a shared store.
Add at the top of the script block:

```typescript
  import { sessionActivity } from "../../stores/sessionActivity.svelte.js";
```

Then in `handleScroll`, after the `requestAnimationFrame` callback fires
(line ~133), add:

```typescript
      // Publish first visible timestamp for minimap sync.
      const visItems = virtualizer.instance?.getVirtualItems() ?? [];
      for (const vi of visItems) {
        const item = displayItemsAsc[
          ui.sortNewestFirst
            ? displayItemsAsc.length - 1 - vi.index
            : vi.index
        ];
        if (!item) continue;
        const ts =
          item.kind === "message"
            ? item.message.timestamp
            : item.timestamp;
        if (ts) {
          sessionActivity.firstVisibleTimestamp = ts;
          break;
        }
      }
```

**Note:** This references the store from Task 9. The import will fail
until Task 9 is complete. That's fine — we commit together.

- [ ] **Step 3: Verify no type errors (aside from missing store import)**

```bash
cd frontend && npx tsc --noEmit 2>&1 | head -20 && cd ..
```

Expected: only the missing `sessionActivity` import error (if Task 9
not yet done).

- [ ] **Step 4: Commit (can be combined with Task 9 commit)**

Hold for Task 9.

---

## Task 9: Create session activity store

**Files:**
- Create: `frontend/src/lib/stores/sessionActivity.svelte.ts`
- Create: `frontend/src/lib/stores/sessionActivity.test.ts`

- [ ] **Step 1: Write the unit test for bucket index derivation**

Create `frontend/src/lib/stores/sessionActivity.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { findActiveBucketIndex } from "./sessionActivity.svelte.js";
import type { SessionActivityBucket } from "../api/types/session-activity.js";

function bucket(
  start: string,
  end: string,
  firstOrdinal: number | null = 0,
): SessionActivityBucket {
  return {
    start_time: start,
    end_time: end,
    user_count: 1,
    assistant_count: 1,
    first_ordinal: firstOrdinal,
  };
}

describe("findActiveBucketIndex", () => {
  const buckets: SessionActivityBucket[] = [
    bucket("2026-03-26T10:00:00Z", "2026-03-26T10:15:00Z", 0),
    bucket("2026-03-26T10:15:00Z", "2026-03-26T10:30:00Z", null),
    bucket("2026-03-26T10:30:00Z", "2026-03-26T10:45:00Z", 10),
  ];

  it("maps timestamp to correct bucket", () => {
    expect(
      findActiveBucketIndex(buckets, "2026-03-26T10:05:00Z"),
    ).toBe(0);
    expect(
      findActiveBucketIndex(buckets, "2026-03-26T10:35:00Z"),
    ).toBe(2);
  });

  it("returns null for null timestamp", () => {
    expect(findActiveBucketIndex(buckets, null)).toBeNull();
  });

  it("returns null for timestamp outside range", () => {
    expect(
      findActiveBucketIndex(buckets, "2026-03-26T09:00:00Z"),
    ).toBeNull();
    expect(
      findActiveBucketIndex(buckets, "2026-03-26T11:00:00Z"),
    ).toBeNull();
  });

  it("maps timestamp at bucket boundary to that bucket", () => {
    expect(
      findActiveBucketIndex(buckets, "2026-03-26T10:15:00Z"),
    ).toBe(1);
  });

  it("returns empty bucket index (for highlight, not click)", () => {
    expect(
      findActiveBucketIndex(buckets, "2026-03-26T10:20:00Z"),
    ).toBe(1);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && npx vitest run src/lib/stores/sessionActivity.test.ts && cd ..
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create `frontend/src/lib/stores/sessionActivity.svelte.ts`**

```typescript
import { getSessionActivity } from "../api/client.js";
import type {
  SessionActivityBucket,
  SessionActivityResponse,
} from "../api/types/session-activity.js";

export function findActiveBucketIndex(
  buckets: SessionActivityBucket[],
  timestamp: string | null,
): number | null {
  if (!timestamp || buckets.length === 0) return null;
  const ts = new Date(timestamp).getTime();
  for (let i = 0; i < buckets.length; i++) {
    const start = new Date(buckets[i]!.start_time).getTime();
    const end = new Date(buckets[i]!.end_time).getTime();
    if (ts >= start && ts < end) return i;
  }
  return null;
}

class SessionActivityStore {
  buckets: SessionActivityBucket[] = $state([]);
  intervalSeconds: number = $state(0);
  totalMessages: number = $state(0);
  loading: boolean = $state(false);
  error: string | null = $state(null);
  firstVisibleTimestamp: string | null = $state(null);

  private cachedSessionId: string | null = null;

  get activeBucketIndex(): number | null {
    return findActiveBucketIndex(
      this.buckets,
      this.firstVisibleTimestamp,
    );
  }

  async load(sessionId: string) {
    if (this.cachedSessionId === sessionId && this.buckets.length > 0) {
      return;
    }
    this.loading = true;
    this.error = null;
    try {
      const resp: SessionActivityResponse =
        await getSessionActivity(sessionId);
      this.buckets = resp.buckets;
      this.intervalSeconds = resp.interval_seconds;
      this.totalMessages = resp.total_messages;
      this.cachedSessionId = sessionId;
    } catch (e) {
      this.error =
        e instanceof Error ? e.message : "Failed to load activity";
      this.buckets = [];
    } finally {
      this.loading = false;
    }
  }

  reload(sessionId: string) {
    this.cachedSessionId = null;
    return this.load(sessionId);
  }

  clear() {
    this.buckets = [];
    this.intervalSeconds = 0;
    this.totalMessages = 0;
    this.loading = false;
    this.error = null;
    this.cachedSessionId = null;
    this.firstVisibleTimestamp = null;
  }
}

export const sessionActivity = new SessionActivityStore();
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && npx vitest run src/lib/stores/sessionActivity.test.ts && cd ..
```

Expected: PASS.

- [ ] **Step 5: Commit (with Task 8 changes)**

```bash
git add frontend/src/lib/stores/sessionActivity.svelte.ts frontend/src/lib/stores/sessionActivity.test.ts frontend/src/lib/stores/ui.svelte.ts frontend/src/lib/components/content/MessageList.svelte
git commit -m "feat: add session activity store, UI toggle state, and scroll timestamp publishing"
```

---

## Task 10: Create `ActivityMinimap.svelte` component

**Files:**
- Create: `frontend/src/lib/components/content/ActivityMinimap.svelte`

- [ ] **Step 1: Create the component**

Create `frontend/src/lib/components/content/ActivityMinimap.svelte`:

```svelte
<script lang="ts">
  import { onMount } from "svelte";
  import { sessionActivity } from "../../stores/sessionActivity.svelte.js";
  import { ui, ALL_BLOCK_TYPES } from "../../stores/ui.svelte.js";
  import type { SessionActivityBucket } from "../../api/types/session-activity.js";

  interface Props {
    sessionId: string;
  }

  let { sessionId }: Props = $props();

  let containerRef: HTMLDivElement | undefined = $state(undefined);
  let containerWidth = $state(400);
  let tooltip = $state<{
    x: number;
    y: number;
    text: string;
  } | null>(null);

  const BAR_HEIGHT = 48;
  const AXIS_HEIGHT = 16;
  const BAR_GAP = 2;
  const MIN_BAR_WIDTH = 4;

  $effect(() => {
    if (sessionId) {
      sessionActivity.load(sessionId);
    }
  });

  onMount(() => {
    if (!containerRef) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width;
      }
    });
    ro.observe(containerRef);
    return () => ro.disconnect();
  });

  let maxCount = $derived.by(() => {
    let max = 0;
    for (const b of sessionActivity.buckets) {
      const total = b.user_count + b.assistant_count;
      if (total > max) max = total;
    }
    return max || 1;
  });

  let barWidth = $derived.by(() => {
    const n = sessionActivity.buckets.length;
    if (n === 0) return MIN_BAR_WIDTH;
    return Math.max(
      MIN_BAR_WIDTH,
      Math.floor((containerWidth - n * BAR_GAP) / n),
    );
  });

  let svgWidth = $derived(
    sessionActivity.buckets.length * (barWidth + BAR_GAP) -
      BAR_GAP,
  );

  function formatTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function handleBarClick(bucket: SessionActivityBucket) {
    if (bucket.first_ordinal == null) return;
    if (ui.hasBlockFilters) {
      ui.showAllBlocks();
    }
    ui.scrollToOrdinal(bucket.first_ordinal);
  }

  function handleBarKeydown(
    e: KeyboardEvent,
    bucket: SessionActivityBucket,
  ) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handleBarClick(bucket);
    }
  }

  function showTooltip(
    e: MouseEvent,
    bucket: SessionActivityBucket,
  ) {
    if (bucket.user_count + bucket.assistant_count === 0) return;
    const rect = (
      e.currentTarget as SVGElement
    ).getBoundingClientRect();
    const containerRect = containerRef?.getBoundingClientRect();
    if (!containerRect) return;
    tooltip = {
      x: rect.left + rect.width / 2 - containerRect.left,
      y: rect.top - containerRect.top - 4,
      text:
        `${formatTime(bucket.start_time)}\u2013${formatTime(bucket.end_time)} \u2014 ` +
        `${bucket.user_count} user, ${bucket.assistant_count} assistant`,
    };
  }

  function hideTooltip() {
    tooltip = null;
  }

  function handleRetry() {
    sessionActivity.reload(sessionId);
  }
</script>

<div class="activity-minimap" bind:this={containerRef}>
  {#if sessionActivity.loading}
    <div class="minimap-status">Loading activity...</div>
  {:else if sessionActivity.error}
    <div class="minimap-status minimap-error">
      {sessionActivity.error}
      <button class="minimap-retry" onclick={handleRetry}>
        Retry
      </button>
    </div>
  {:else if sessionActivity.buckets.length === 0}
    <div class="minimap-status">
      No timestamp data available
    </div>
  {:else}
    <div class="minimap-chart">
      <svg
        width={svgWidth}
        height={BAR_HEIGHT}
        role="group"
        aria-label="Session activity timeline"
      >
        {#each sessionActivity.buckets as bucket, i}
          {@const total = bucket.user_count + bucket.assistant_count}
          {@const isEmpty = total === 0}
          {@const userH =
            (bucket.user_count / maxCount) * BAR_HEIGHT}
          {@const asstH =
            (bucket.assistant_count / maxCount) * BAR_HEIGHT}
          {@const x = i * (barWidth + BAR_GAP)}
          {@const isActive =
            sessionActivity.activeBucketIndex === i}
          <g
            class="minimap-bar"
            class:minimap-bar--active={isActive}
            class:minimap-bar--empty={isEmpty}
            class:minimap-bar--clickable={!isEmpty}
            onclick={() => handleBarClick(bucket)}
            onkeydown={(e) => handleBarKeydown(e, bucket)}
            onmouseenter={(e) => showTooltip(e, bucket)}
            onmouseleave={hideTooltip}
            tabindex={isEmpty ? -1 : 0}
            role={isEmpty ? "presentation" : "button"}
            aria-label={isEmpty
              ? undefined
              : `${formatTime(bucket.start_time)}\u2013${formatTime(bucket.end_time)}: ${bucket.user_count} user, ${bucket.assistant_count} assistant`}
          >
            {#if isEmpty}
              <rect
                {x}
                y={BAR_HEIGHT - 2}
                width={barWidth}
                height={2}
                class="bar-empty"
              />
            {:else}
              <rect
                {x}
                y={BAR_HEIGHT - asstH - userH}
                width={barWidth}
                height={userH}
                class="bar-user"
                rx="1"
              />
              <rect
                {x}
                y={BAR_HEIGHT - asstH}
                width={barWidth}
                height={asstH}
                class="bar-assistant"
                rx="1"
              />
            {/if}
            {#if isActive}
              <rect
                x={x - 1}
                y={BAR_HEIGHT - asstH - userH - 1}
                width={barWidth + 2}
                height={(isEmpty ? 2 : asstH + userH) + 2}
                class="bar-indicator"
                rx="2"
              />
            {/if}
          </g>
        {/each}
      </svg>
      {#if tooltip}
        <div
          class="minimap-tooltip"
          style:left="{tooltip.x}px"
          style:top="{tooltip.y}px"
        >
          {tooltip.text}
        </div>
      {/if}
    </div>
    <div class="minimap-axis">
      <span class="minimap-time">
        {formatTime(sessionActivity.buckets[0]!.start_time)}
      </span>
      <span class="minimap-legend">
        <span class="legend-swatch legend-user"></span> user
        <span class="legend-swatch legend-assistant"></span>
        assistant
      </span>
      <span class="minimap-time">
        {formatTime(
          sessionActivity.buckets[
            sessionActivity.buckets.length - 1
          ]!.end_time,
        )}
      </span>
    </div>
  {/if}
</div>

<style>
  .activity-minimap {
    position: relative;
    padding: 6px 14px 4px;
    border-bottom: 1px solid var(--border-muted);
  }

  .minimap-status {
    font-size: 10px;
    color: var(--text-muted);
    padding: 4px 0;
  }

  .minimap-error {
    color: var(--accent-red, #e55);
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .minimap-retry {
    font-size: 10px;
    color: var(--accent-blue);
    cursor: pointer;
    background: none;
    border: none;
    text-decoration: underline;
    padding: 0;
  }

  .minimap-chart {
    position: relative;
    overflow-x: auto;
    overflow-y: hidden;
  }

  .minimap-chart svg {
    display: block;
    width: 100%;
  }

  .bar-user {
    fill: var(--accent-green, #3fb950);
  }

  .bar-assistant {
    fill: var(--accent-blue, #58a6ff);
  }

  .bar-empty {
    fill: var(--border-muted);
  }

  .bar-indicator {
    fill: none;
    stroke: var(--accent-blue, #58a6ff);
    stroke-width: 1.5;
  }

  .minimap-bar--clickable {
    cursor: pointer;
  }

  .minimap-bar--clickable:hover .bar-user {
    opacity: 0.8;
  }

  .minimap-bar--clickable:hover .bar-assistant {
    opacity: 0.8;
  }

  .minimap-bar:focus-visible {
    outline: none;
  }

  .minimap-bar:focus-visible .bar-indicator {
    fill: none;
    stroke: var(--accent-blue, #58a6ff);
    stroke-width: 2;
  }

  .minimap-tooltip {
    position: absolute;
    transform: translateX(-50%) translateY(-100%);
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: 4px;
    padding: 3px 8px;
    font-size: 10px;
    color: var(--text-primary);
    white-space: nowrap;
    pointer-events: none;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
    z-index: 10;
  }

  .minimap-axis {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: 3px;
    font-size: 9px;
    color: var(--text-muted);
  }

  .minimap-time {
    font-variant-numeric: tabular-nums;
  }

  .minimap-legend {
    display: flex;
    align-items: center;
    gap: 3px;
  }

  .legend-swatch {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 2px;
  }

  .legend-user {
    background: var(--accent-green, #3fb950);
  }

  .legend-assistant {
    background: var(--accent-blue, #58a6ff);
    margin-left: 6px;
  }
</style>
```

- [ ] **Step 2: Verify no type errors**

```bash
cd frontend && npx tsc --noEmit && cd ..
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/components/content/ActivityMinimap.svelte
git commit -m "feat: add ActivityMinimap SVG bar chart component"
```

---

## Task 11: Wire toggle button and minimap into session detail view

**Files:**
- Modify: `frontend/src/lib/components/layout/SessionBreadcrumb.svelte`
- Modify: `frontend/src/App.svelte`

- [ ] **Step 1: Add toggle button to `SessionBreadcrumb.svelte`**

In the script block, add the import:

```typescript
  import { ui } from "../../stores/ui.svelte.js";
```

In the template, inside `.actions-wrapper` (before the `find-btn`
button, around line 497), add:

```svelte
        <button
          class="minimap-btn"
          class:minimap-btn--active={ui.activityMinimapOpen}
          title="Activity minimap"
          onclick={() => ui.toggleActivityMinimap()}
          aria-label="Toggle activity minimap"
        >
          <svg width="13" height="13" viewBox="0 0 16 16" fill="currentColor">
            <path d="M1 14V8h2v6H1zm4 0V2h2v12H5zm4 0V5h2v9H9zm4 0V9h2v5h-2z"/>
          </svg>
        </button>
```

Add CSS styles (alongside `.find-btn`):

```css
  .minimap-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: var(--radius-sm, 4px);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
    flex-shrink: 0;
  }

  .minimap-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--accent-blue);
  }

  .minimap-btn--active {
    color: var(--accent-blue);
    background: color-mix(
      in srgb,
      var(--accent-blue) 12%,
      transparent
    );
  }
```

- [ ] **Step 2: Insert `ActivityMinimap` in `App.svelte`**

In the `content` snippet (around line 382), between
`SessionBreadcrumb` and `MessageList`:

```svelte
        <SessionBreadcrumb
          session={session}
          onBack={() => sessions.deselectSession()}
        />
        {#if ui.activityMinimapOpen && sessions.activeSessionId}
          <ActivityMinimap
            sessionId={sessions.activeSessionId}
          />
        {/if}
        <MessageList bind:this={messageListRef} />
```

Add the import at the top of the script block:

```typescript
  import ActivityMinimap from "./lib/components/content/ActivityMinimap.svelte";
```

- [ ] **Step 3: Handle SSE refresh**

In `App.svelte`, find where SSE session updates trigger message
reloads. Add a minimap refresh alongside it. Look for the SSE event
handler and add:

```typescript
import { sessionActivity } from "./lib/stores/sessionActivity.svelte.js";

// Inside the SSE update handler, after message refresh:
if (ui.activityMinimapOpen) {
  sessionActivity.reload(sessionId);
}
```

Also clear the store when session changes:

```typescript
// In the session change effect:
sessionActivity.clear();
```

- [ ] **Step 4: Build and test manually**

```bash
cd frontend && npm run build && cd ..
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/layout/SessionBreadcrumb.svelte frontend/src/App.svelte
git commit -m "feat: wire activity minimap toggle and component into session detail view"
```

---

## Task 12: E2E test

**Files:**
- Modify: existing E2E test file or create new spec

- [ ] **Step 1: Check existing E2E structure**

```bash
ls frontend/e2e/
```

Identify the test file pattern and test fixture setup.

- [ ] **Step 2: Add E2E test for minimap**

Create or append to an appropriate E2E spec:

```typescript
import { test, expect } from "@playwright/test";

test("activity minimap toggles and navigates", async ({
  page,
}) => {
  // Navigate to a session with messages.
  await page.goto("/");
  await page.locator(".session-item").first().click();

  // Minimap should be hidden by default.
  await expect(
    page.locator(".activity-minimap"),
  ).not.toBeVisible();

  // Click the toggle button.
  await page.locator(".minimap-btn").click();

  // Minimap should now be visible.
  await expect(
    page.locator(".activity-minimap"),
  ).toBeVisible();

  // Should have SVG bars.
  const bars = page.locator(".minimap-bar");
  await expect(bars.first()).toBeVisible();

  // Click a populated bar.
  const clickableBar = page.locator(
    ".minimap-bar--clickable",
  ).first();
  await clickableBar.click();

  // Toggle off.
  await page.locator(".minimap-btn").click();
  await expect(
    page.locator(".activity-minimap"),
  ).not.toBeVisible();
});
```

- [ ] **Step 3: Run E2E test**

```bash
cd frontend && npx playwright test --grep "activity minimap" && cd ..
```

- [ ] **Step 4: Commit**

```bash
git add frontend/e2e/
git commit -m "test: add E2E test for activity minimap toggle and navigation"
```
