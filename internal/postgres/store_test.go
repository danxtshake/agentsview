//go:build pgtest

package postgres

import (
	"context"
	"testing"

	"github.com/wesm/agentsview/internal/db"
)

const testSchema = "agentsview_store_test"

// ensureStoreSchema creates the test schema and seed data.
func ensureStoreSchema(t *testing.T, pgURL string) {
	t.Helper()
	pg, err := Open(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("connecting to pg: %v", err)
	}
	defer pg.Close()

	_, err = pg.Exec(`
		DROP SCHEMA IF EXISTS ` + testSchema + ` CASCADE;
	`)
	if err != nil {
		t.Fatalf("dropping schema: %v", err)
	}

	ctx := context.Background()
	if err := EnsureSchema(ctx, pg, testSchema); err != nil {
		t.Fatalf("creating schema: %v", err)
	}

	_, err = pg.Exec(`
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 started_at, ended_at, message_count,
			 user_message_count)
		VALUES
			('store-test-001', 'test-machine',
			 'test-project', 'claude-code',
			 'hello world',
			 '2026-03-12T10:00:00Z'::timestamptz,
			 '2026-03-12T10:30:00Z'::timestamptz,
			 2, 1)
	`)
	if err != nil {
		t.Fatalf("inserting test session: %v", err)
	}
	_, err = pg.Exec(`
		INSERT INTO messages
			(session_id, ordinal, role, content,
			 timestamp, content_length)
		VALUES
			('store-test-001', 0, 'user',
			 'hello world',
			 '2026-03-12T10:00:00Z'::timestamptz, 11),
			('store-test-001', 1, 'assistant',
			 'hi there',
			 '2026-03-12T10:00:01Z'::timestamptz, 8)
	`)
	if err != nil {
		t.Fatalf("inserting test messages: %v", err)
	}
}

func TestNewStore(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if !store.ReadOnly() {
		t.Error("ReadOnly() = false, want true")
	}
	if !store.HasFTS() {
		t.Error("HasFTS() = false, want true")
	}
}

func TestStoreListSessions(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	page, err := store.ListSessions(
		ctx, db.SessionFilter{Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if page.Total == 0 {
		t.Error("expected at least 1 session")
	}
	t.Logf("sessions: %d, total: %d",
		len(page.Sessions), page.Total)
}

func TestStoreListSessions_MachineMultiSelect(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	_, err = store.DB().Exec(`
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 started_at, ended_at, message_count,
			 user_message_count)
		VALUES
			('store-test-002', 'machine-b',
			 'test-project', 'codex',
			 'hello machine b',
			 '2026-03-12T11:00:00Z'::timestamptz,
			 '2026-03-12T11:30:00Z'::timestamptz,
			 2, 1),
			('store-test-003', 'machine-c',
			 'test-project', 'gemini',
			 'hello machine c',
			 '2026-03-12T12:00:00Z'::timestamptz,
			 '2026-03-12T12:30:00Z'::timestamptz,
			 2, 1)
	`)
	if err != nil {
		t.Fatalf("inserting extra sessions: %v", err)
	}

	ctx := context.Background()
	page, err := store.ListSessions(
		ctx,
		db.SessionFilter{
			Machine: "test-machine,machine-c",
			Limit:   10,
		},
	)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("total = %d, want 2", page.Total)
	}
	got := []string{
		page.Sessions[0].Machine,
		page.Sessions[1].Machine,
	}
	if got[0] != "test-machine" && got[1] != "test-machine" {
		t.Fatalf("machines = %v, want test-machine included", got)
	}
	if got[0] != "machine-c" && got[1] != "machine-c" {
		t.Fatalf("machines = %v, want machine-c included", got)
	}
}

func TestStoreGetSession(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	sess, err := store.GetSession(ctx, "store-test-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Project != "test-project" {
		t.Errorf("project = %q, want %q",
			sess.Project, "test-project")
	}
}

func TestStoreGetMessages(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	msgs, err := store.GetMessages(
		ctx, "store-test-001", 0, 100, true,
	)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("got %d messages, want 2", len(msgs))
	}
}

func TestStoreGetStats(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	stats, err := store.GetStats(ctx, false)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.SessionCount == 0 {
		t.Error("expected at least 1 session in stats")
	}
	t.Logf("stats: %+v", stats)
}

func TestStoreSearch(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	page, err := store.Search(ctx, db.SearchFilter{
		Query: "hello",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) == 0 {
		t.Error("expected at least 1 search result")
	}
	t.Logf("search results: %d", len(page.Results))
}

func TestStoreAnalyticsSummary(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	summary, err := store.GetAnalyticsSummary(
		ctx, db.AnalyticsFilter{
			From: "2026-01-01",
			To:   "2026-12-31",
		},
	)
	if err != nil {
		t.Fatalf("GetAnalyticsSummary: %v", err)
	}
	if summary.TotalSessions == 0 {
		t.Error("expected at least 1 session in summary")
	}
	t.Logf("summary: %+v", summary)
}

func TestStoreGetSessionActivity_FractionalTimestamps(
	t *testing.T,
) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	sid := "store-test-frac-ts"
	_, err = store.DB().Exec(`
		DELETE FROM messages WHERE session_id = $1;
		DELETE FROM sessions WHERE id = $1;
		INSERT INTO sessions
			(id, machine, project, agent, first_message,
			 started_at, ended_at, message_count,
			 user_message_count)
		VALUES
			($1, 'test-machine', 'test-project',
			 'claude', 'frac test',
			 '2026-03-26T10:00:00Z'::timestamptz,
			 '2026-03-26T10:02:00Z'::timestamptz,
			 3, 2);
		INSERT INTO messages
			(session_id, ordinal, role, content,
			 timestamp, content_length)
		VALUES
			($1, 0, 'user', 'a',
			 '2026-03-26T10:00:00.900Z'::timestamptz, 1),
			($1, 1, 'assistant', 'b',
			 '2026-03-26T10:00:59.100Z'::timestamptz, 1),
			($1, 2, 'user', 'c',
			 '2026-03-26T10:01:01.000Z'::timestamptz, 1)
	`, sid)
	if err != nil {
		t.Fatalf("inserting test data: %v", err)
	}

	ctx := context.Background()
	resp, err := store.GetSessionActivity(ctx, sid)
	if err != nil {
		t.Fatalf("GetSessionActivity: %v", err)
	}

	if resp.IntervalSeconds != 60 {
		t.Fatalf(
			"interval = %d, want 60",
			resp.IntervalSeconds,
		)
	}

	if len(resp.Buckets) < 2 {
		t.Fatalf(
			"buckets = %d, want >= 2",
			len(resp.Buckets),
		)
	}

	// First bucket should have both sub-second messages.
	first := resp.Buckets[0]
	if first.UserCount != 1 || first.AssistantCount != 1 {
		t.Errorf(
			"first bucket: user=%d asst=%d, want 1,1",
			first.UserCount, first.AssistantCount,
		)
	}

	// Second bucket should have the third message.
	second := resp.Buckets[1]
	if second.UserCount != 1 {
		t.Errorf(
			"second bucket user=%d, want 1",
			second.UserCount,
		)
	}
}

func TestStoreWriteMethodsReturnReadOnly(t *testing.T) {
	pgURL := testPGURL(t)

	store, err := NewStore(pgURL, testSchema, true)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"StarSession", func() error {
			_, err := store.StarSession("x")
			return err
		}},
		{"UnstarSession", func() error {
			return store.UnstarSession("x")
		}},
		{"BulkStarSessions", func() error {
			return store.BulkStarSessions([]string{"x"})
		}},
		{"PinMessage", func() error {
			_, err := store.PinMessage("x", 1, nil)
			return err
		}},
		{"UnpinMessage", func() error {
			return store.UnpinMessage("x", 1)
		}},
		{"InsertInsight", func() error {
			_, err := store.InsertInsight(db.Insight{})
			return err
		}},
		{"DeleteInsight", func() error {
			return store.DeleteInsight(1)
		}},
		{"RenameSession", func() error {
			return store.RenameSession("x", nil)
		}},
		{"SoftDeleteSession", func() error {
			return store.SoftDeleteSession("x")
		}},
		{"RestoreSession", func() error {
			_, err := store.RestoreSession("x")
			return err
		}},
		{"DeleteSessionIfTrashed", func() error {
			_, err := store.DeleteSessionIfTrashed("x")
			return err
		}},
		{"EmptyTrash", func() error {
			_, err := store.EmptyTrash()
			return err
		}},
		{"UpsertSession", func() error {
			return store.UpsertSession(db.Session{})
		}},
		{"ReplaceSessionMessages", func() error {
			return store.ReplaceSessionMessages("x", nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != db.ErrReadOnly {
				t.Errorf("got %v, want ErrReadOnly", err)
			}
		})
	}
}
