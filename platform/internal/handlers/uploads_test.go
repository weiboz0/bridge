package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newUploadHandler(t *testing.T) (*UploadHandler, string) {
	t.Helper()
	dir := t.TempDir()
	return &UploadHandler{UploadDir: dir}, dir
}

// createMultipartFile creates a multipart form body with a "file" field.
func createMultipartFile(t *testing.T, filename string, content []byte, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="` + filename + `"`}
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}
	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write(content)
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()

	return &body, writer.FormDataContentType()
}

// A minimal valid PNG file (1x1 pixel).
var minimalPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, // bit depth, color type, etc.
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, // IDAT chunk
	0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC,
	0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, // IEND chunk
	0x44, 0xAE, 0x42, 0x60, 0x82,
}

func TestUploadHandler_Upload_Success(t *testing.T) {
	h, dir := newUploadHandler(t)

	body, contentType := createMultipartFile(t, "test.png", minimalPNG, "image/png")

	req := httptest.NewRequest("POST", "/api/uploads", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	h.Upload(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusCreated {
		resBody, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 201, got %d: %s", res.StatusCode, string(resBody))
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.HasPrefix(result.URL, "/api/uploads/") {
		t.Fatalf("expected URL starting with /api/uploads/, got %s", result.URL)
	}
	if !strings.HasSuffix(result.URL, ".png") {
		t.Fatalf("expected URL ending with .png, got %s", result.URL)
	}

	// Verify file exists on disk.
	filename := filepath.Base(result.URL)
	if _, err := os.Stat(filepath.Join(dir, filename)); err != nil {
		t.Fatalf("uploaded file not found on disk: %v", err)
	}
}

func TestUploadHandler_Upload_MissingFile(t *testing.T) {
	h, _ := newUploadHandler(t)

	req := httptest.NewRequest("POST", "/api/uploads", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	w := httptest.NewRecorder()

	h.Upload(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestUploadHandler_Upload_UnsupportedType(t *testing.T) {
	h, _ := newUploadHandler(t)

	// A text file is not in the allowed MIME types.
	body, contentType := createMultipartFile(t, "test.txt", []byte("hello world"), "text/plain")

	req := httptest.NewRequest("POST", "/api/uploads", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	h.Upload(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		resBody, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 400, got %d: %s", res.StatusCode, string(resBody))
	}
}

func TestUploadHandler_Serve_Success(t *testing.T) {
	h, dir := newUploadHandler(t)

	// Write a file to the upload directory.
	filename := "testfile.png"
	if err := os.WriteFile(filepath.Join(dir, filename), minimalPNG, 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/api/uploads/{filename}", h.Serve)

	req := httptest.NewRequest("GET", "/api/uploads/"+filename, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "image/png") {
		t.Fatalf("expected image/png content type, got %s", ct)
	}
}

func TestUploadHandler_Serve_NotFound(t *testing.T) {
	h, _ := newUploadHandler(t)

	r := chi.NewRouter()
	r.Get("/api/uploads/{filename}", h.Serve)

	req := httptest.NewRequest("GET", "/api/uploads/nonexistent.png", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.StatusCode)
	}
}

func TestUploadHandler_Serve_DirectoryTraversal(t *testing.T) {
	h, _ := newUploadHandler(t)

	r := chi.NewRouter()
	r.Get("/api/uploads/{filename}", h.Serve)

	// Attempt directory traversal.
	req := httptest.NewRequest("GET", "/api/uploads/..%2F..%2Fetc%2Fpasswd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	res := w.Result()
	// Should be 400 or 404 — never serve the file.
	if res.StatusCode == http.StatusOK {
		t.Fatal("expected non-200 for directory traversal attempt")
	}
}
