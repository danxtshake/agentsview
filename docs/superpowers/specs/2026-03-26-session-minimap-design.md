# Session Activity Minimap

A togglable horizontal bar chart showing message activity intensity over
the timeline of a session. Stacked bars distinguish user vs assistant
messages per time interval. Clicking a bar scrolls the message list to
that point in the session.

## API

### New endpoint

`GET /api/v1/sessions/{id}/activity`

Returns time-bucketed message counts with adaptive interval sizing.
Only visible messages are counted — both persisted system messages
(`is_system = 1`) and prefix-detected injected user messages are
excluded. The activity query reuses the same backend filtering helper
used by search (`internal/db/search.go`), which handles both cases.

```json
{
  "buckets": [
    {
      "start_time": "2026-03-26T10:00:00Z",
      "end_time": "2026-03-26T10:15:00Z",
      "user_count": 4,
      "assistant_count": 5,
      "first_ordinal": 12
    }
  ],
  "interval_seconds": 900,
  "total_messages": 142
}
```

**Types**: `SessionActivityResponse` (top-level) and
`SessionActivityBucket` (per-bucket). Avoids collision with the existing
`ActivityResponse` in the analytics API.

Empty buckets (gaps with no messages) are included in the response with
zero counts and `first_ordinal: null`. This lets the frontend render
explicit gaps. Empty buckets are non-clickable; the "you are here"
indicator skips them.

### Adaptive interval logic

Target 20-40 buckets. Session duration is defined as
`max(timestamp) - min(timestamp)` over visible (non-system,
non-injected) messages with non-NULL timestamps. Compute
`session_duration / 30` to get a base interval, then snap to the
nearest value in a fixed set:
`[1m, 2m, 5m, 10m, 15m, 30m, 1h, 2h]`.

- Sessions under 10 minutes: 1-minute buckets.
- Sessions over 16 hours: 2-hour buckets.

### SQL implementation

SQLite and PostgreSQL require separate queries due to different
timestamp storage:

- **SQLite**: `messages.timestamp` is nullable TEXT (RFC 3339). Convert
  via `CAST(strftime('%s', timestamp) AS INTEGER)` before dividing by
  interval. Rows with NULL or unparseable timestamps are excluded from
  bucketing (they still count toward `total_messages` so the user knows
  data exists, but they cannot be placed on the timeline).
- **PostgreSQL**: `messages.timestamp` is `TIMESTAMPTZ`. Use
  `EXTRACT(EPOCH FROM timestamp)::bigint` for bucketing.

`first_ordinal` is `MIN(ordinal)` per bucket within the filtered
visible-message set (same exclusions as above), ensuring click-to-scroll
never lands on a hidden message.

**Timestamp edge cases**:

- **NULL timestamps**: Excluded from buckets. If all messages lack
  timestamps, the endpoint returns an empty `buckets` array. The
  frontend shows the minimap area with an inline "No timestamp data
  available" message (same no-data state as described in the component
  section).
- **Skewed/duplicate timestamps**: Bucketing groups by wall-clock time
  as stored. Ordinal order remains authoritative for scroll targets —
  `first_ordinal` is always the lowest ordinal in the bucket regardless
  of timestamp ordering.

### Cleanup: remove unused minimap endpoint

The existing `/api/v1/sessions/{id}/minimap` endpoint and all supporting
code is unused. Removal checklist (implementation should grep for
remaining references and include any not listed here):

**Backend (Go)**:

- `internal/server/messages.go`: `handleGetMinimap` handler
- `internal/server/server.go`: `/minimap` route registration
- `internal/server/server_test.go`: minimap-related test cases
- `internal/server/deadline_test.go`: minimap deadline test cases
- `internal/server/deadline_internal_test.go`: minimap internal tests
- `internal/db/db_test.go`: minimap database test cases
- `internal/postgres/store_test.go`: minimap mock/interface tests
- `internal/db/messages.go`: `GetMinimap`, `GetMinimapFrom`,
  `SampleMinimap` functions
- `internal/db/store.go`: `GetMinimap` interface method
- `internal/postgres/messages.go`: `GetMinimap` implementation

**Frontend (TypeScript)**:

- `frontend/src/lib/api/client.ts`: `getMinimap` method
- `frontend/src/lib/api/types/core.ts`: `MinimapEntry`,
  `MinimapResponse` types
- `frontend/src/lib/stores/messages.test.ts`: minimap-related test
  imports/cases

## Frontend

### Charting approach: custom SVG

Custom SVG following the existing pattern in `ActivityTimeline.svelte`.
The repo already has hand-rolled SVG chart components with resize
handling, tooltips, click targets, and keyboard behavior. For a 20-40
bar minimap this avoids a new dependency and stays consistent with the
codebase. A charting library (Observable Plot) can be reconsidered when
a future feature genuinely requires it.

### Toggle button

A bar-chart icon button added to the `SessionBreadcrumb` controls area,
alongside the existing find and actions buttons. Blue highlight when
active, matching the `find-btn--active` pattern.

### Component: `ActivityMinimap.svelte`

Renders between `SessionBreadcrumb` and `MessageList` when toggled on.

- **Stacked bar chart**: User messages (green, `--accent-green`) stacked
  on top of assistant messages (blue, `--accent-blue`). Heights
  proportional to count relative to the max bucket total.
- **Fixed height**: ~80px total including time axis and legend.
- **Empty intervals**: Rendered as a thin gray baseline so gaps in
  activity are visible. Non-clickable, no tooltip.
- **Time axis**: Start and end times at edges.
- **Inline legend**: Small colored squares with "user" / "assistant"
  labels, positioned between the time labels.
- **Hover tooltip**: Time range and counts (e.g.,
  "10:00-10:15 — 4 user, 5 assistant"). Populated buckets only.
- **Click**: Sets `ui.pendingScrollOrdinal` to the bucket's
  `first_ordinal`, scrolling the message list to that point. If active
  block filters (user/assistant/tool/code visibility toggles) would
  hide the target message, the scroll resets filters to show all blocks
  before navigating — matching how find-in-session already handles
  filtered results. Empty buckets are inert.
- **Active indicator**: Timestamp-based, not ordinal-based.
  `MessageList` publishes `firstVisibleTimestamp: string | null` into a
  shared store on each scroll frame (derived from the first visible
  display item that carries a timestamp). The minimap maps this value to
  the bucket whose `[start_time, end_time)` range contains it. If
  `firstVisibleTimestamp` is null, the indicator is cleared (no bucket
  highlighted). This avoids ambiguity with empty buckets and is stable
  regardless of display-item grouping or block filtering.
- **Responsive**: Bars stretch to fill available width. Bar count is
  determined by the server.

### Error and accessibility

- **Error state**: `error: string | null` in the store. On fetch
  failure, the minimap area shows a single-line inline error with a
  retry button. The toggle button remains active so the user can dismiss
  or retry.
- **Keyboard**: Each populated bar is focusable (`tabindex="0"`) and
  activatable via Enter/Space. Empty bars are skipped in tab order.
  Arrow keys move focus between bars.
- **ARIA**: Bars have `role="button"` and `aria-label` describing the
  time range and counts.
- **No-data state**: If the fetch returns empty buckets (no timestamped
  messages), the minimap area shows a brief inline message ("No
  timestamp data available") and the user can dismiss via the toggle.
  The toggle button is never pre-disabled — there is no session metadata
  to determine timestamp coverage before fetching.

### Store: `sessionActivityMinimap.svelte.ts`

Small dedicated Svelte 5 store:

- `buckets: SessionActivityBucket[]` — API response data.
- `intervalSeconds: number` — bucket width.
- `loading: boolean` — fetch state.
- `error: string | null` — fetch error.
- `visible: boolean` — toggle state, persisted via UI store.
- `activeBucketIndex: number | null` — derived from the first visible
  message with a timestamp, mapped to the bucket whose
  `[start_time, end_time)` range contains it. Cleared if no visible
  message has a timestamp.

### Data flow

1. User clicks the toggle button. `visible` becomes `true`.
2. Store fetches `/api/v1/sessions/{id}/activity` (once per session,
   cached until session changes). On failure, sets `error`.
3. Custom SVG renders the stacked bar chart from `buckets`.
4. On scroll, `MessageList` publishes `firstVisibleTimestamp` into a
   shared store. The minimap derives `activeBucketIndex` by comparing
   this timestamp against bucket time ranges.
5. On SSE session update, the store refetches activity data.
6. Clicking a populated bar sets `ui.pendingScrollOrdinal`, reusing the
   existing scroll-to-ordinal mechanism in `MessageList`.

## Testing

### Backend

- Table-driven tests for the activity bucketing query: correct grouping,
  role counting, `first_ordinal` accuracy, system message exclusion.
- Adaptive interval selection: verify snapping logic for various session
  durations.
- Edge cases: single message, zero messages, messages spanning midnight,
  very long sessions (>24h), NULL timestamps excluded, all-NULL
  timestamps returning empty buckets.
- Separate test cases for SQLite and PostgreSQL query paths.

### Frontend

- Unit test for `activeBucketIndex` derivation (timestamp-to-bucket
  mapping).
- Unit test for empty bucket handling (non-clickable, skipped in active
  indicator).
- E2E test: load a session with known message distribution, toggle
  minimap on, verify bars render, click a bar, verify scroll position
  changes.
- Test error state: mock a failed fetch, verify error message and retry
  button.
- Test the data transformation that feeds the chart, not the chart
  rendering itself.
