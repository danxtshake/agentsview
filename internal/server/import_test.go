package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/importer"
)

func TestHandleImportClaudeAI(t *testing.T) {
	srv := testServer(t, 5*time.Second)

	conversations := `[
      {
        "uuid": "api-test-001",
        "name": "API Test",
        "summary": "",
        "created_at": "2026-03-01T10:00:00.000000Z",
        "updated_at": "2026-03-01T10:05:00.000000Z",
        "account": {"uuid": "acct-1"},
        "chat_messages": [
          {
            "uuid": "m1",
            "text": "Test message",
            "content": [{"type":"text","text":"Test message"}],
            "sender": "human",
            "created_at": "2026-03-01T10:00:00.000000Z",
            "updated_at": "2026-03-01T10:00:00.000000Z",
            "attachments": [],
            "files": []
          }
        ]
      }
    ]`

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "conversations.json")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	_, _ = part.Write([]byte(conversations))
	writer.Close()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/import/claude-ai",
		&body,
	)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want 200: %s",
			rec.Code, rec.Body.String(),
		)
	}

	var stats importer.ImportStats
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if stats.Imported != 1 {
		t.Errorf("imported = %d, want 1", stats.Imported)
	}
	if stats.Updated != 0 {
		t.Errorf("updated = %d, want 0", stats.Updated)
	}
}

func TestHandleImportChatGPT_RequiresZip(t *testing.T) {
	srv := testServer(t, 5*time.Second)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "data.json")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	_, _ = part.Write([]byte("[]"))
	writer.Close()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/import/chatgpt",
		&body,
	)
	req.Header.Set(
		"Content-Type", writer.FormDataContentType(),
	)

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want 400: %s",
			rec.Code, rec.Body.String(),
		)
	}
}

func TestHandleImportClaudeAI_NoFile(t *testing.T) {
	srv := testServer(t, 5*time.Second)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.Close()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/import/claude-ai",
		&body,
	)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want 400: %s",
			rec.Code, rec.Body.String(),
		)
	}
}
