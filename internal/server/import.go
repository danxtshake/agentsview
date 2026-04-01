package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wesm/agentsview/internal/importer"
)

func (s *Server) handleImportClaudeAI(
	w http.ResponseWriter, r *http.Request,
) {
	if s.db.ReadOnly() {
		writeError(w, http.StatusNotImplemented,
			"import not available in read-only mode")
		return
	}

	// 64 MB max memory for multipart parsing.
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest,
			"invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"missing 'file' field in form data")
		return
	}
	defer file.Close()

	var reader io.Reader = file

	// Handle zip uploads.
	if strings.HasSuffix(
		strings.ToLower(header.Filename), ".zip",
	) {
		tmpFile, tmpErr := os.CreateTemp(
			"", "claude-import-*.zip",
		)
		if tmpErr != nil {
			writeError(w, http.StatusInternalServerError,
				"failed to create temp file")
			return
		}
		defer os.Remove(tmpFile.Name())

		if _, tmpErr = io.Copy(tmpFile, file); tmpErr != nil {
			tmpFile.Close()
			writeError(w, http.StatusInternalServerError,
				"failed to save upload")
			return
		}
		tmpFile.Close()

		dir, cleanup, extractErr := importer.ExtractZip(
			tmpFile.Name(),
		)
		if extractErr != nil {
			writeError(w, http.StatusBadRequest,
				"failed to extract zip: "+extractErr.Error())
			return
		}
		defer cleanup()

		jsonPath := filepath.Join(dir, "conversations.json")
		jsonFile, openErr := os.Open(jsonPath)
		if openErr != nil {
			writeError(w, http.StatusBadRequest,
				"no conversations.json found in zip")
			return
		}
		defer jsonFile.Close()
		reader = jsonFile
	}

	stats, err := importer.ImportClaudeAI(
		r.Context(), s.db, reader, nil,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleImportChatGPT(
	w http.ResponseWriter, r *http.Request,
) {
	if s.db.ReadOnly() {
		writeError(w, http.StatusNotImplemented,
			"import not available in read-only mode")
		return
	}

	// 256 MB max memory for multipart parsing.
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeError(w, http.StatusBadRequest,
			"invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"missing 'file' field in form data")
		return
	}
	defer file.Close()

	if !strings.HasSuffix(
		strings.ToLower(header.Filename), ".zip",
	) {
		writeError(w, http.StatusBadRequest,
			"ChatGPT import requires a .zip file")
		return
	}

	tmpFile, err := os.CreateTemp("", "chatgpt-import-*.zip")
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"failed to create temp file")
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err = io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeError(w, http.StatusInternalServerError,
			"failed to save upload")
		return
	}
	tmpFile.Close()

	dir, cleanup, err := importer.ExtractZip(tmpFile.Name())
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"failed to extract zip: "+err.Error())
		return
	}
	defer cleanup()

	assetsDir := filepath.Join(s.cfg.DataDir, "assets")
	stats, err := importer.ImportChatGPT(
		r.Context(), s.db, dir, assetsDir, nil,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
