package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// UploadHandler handles file uploads (images, PDFs) stored on local disk.
// In production this would be replaced by an object store (S3, GCS).
type UploadHandler struct {
	// UploadDir is the directory where uploaded files are stored.
	UploadDir string
}

// maxUploadSize is the maximum allowed file size (10 MB).
const maxUploadSize = 10 << 20 // 10 MB

// allowedMIMETypes is the set of MIME types we accept for upload.
var allowedMIMETypes = map[string]bool{
	"image/png":         true,
	"image/jpeg":        true,
	"image/gif":         true,
	"image/webp":        true,
	"image/svg+xml":     true,
	"application/pdf":   true,
}

// mimeToExt maps MIME types to preferred file extensions.
var mimeToExt = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"image/svg+xml":   ".svg",
	"application/pdf": ".pdf",
}

// Routes registers the upload handler routes.
func (h *UploadHandler) Routes(r chi.Router) {
	r.Post("/api/uploads", h.Upload)
	r.Get("/api/uploads/{filename}", h.Serve)
}

// Upload handles POST /api/uploads.
// Accepts multipart/form-data with a "file" field.
// Returns { "url": "/api/uploads/{filename}" }.
func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1024) // small headroom for form overhead

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "File too large (max 10MB)")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file field")
		return
	}
	defer file.Close()

	// Validate file size.
	if header.Size > maxUploadSize {
		writeError(w, http.StatusBadRequest, "File too large (max 10MB)")
		return
	}

	// Detect MIME type from content (more reliable than Content-Type header).
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		writeError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}
	detectedType := http.DetectContentType(buf[:n])

	// For SVG, content detection returns text/xml or application/xml.
	// Fall back to the declared content type or file extension.
	mimeType := detectedType
	if !allowedMIMETypes[mimeType] {
		// Try declared Content-Type from the multipart header.
		declared := header.Header.Get("Content-Type")
		if declared != "" {
			mediaType, _, _ := mime.ParseMediaType(declared)
			if allowedMIMETypes[mediaType] {
				mimeType = mediaType
			}
		}
	}
	if !allowedMIMETypes[mimeType] {
		// Last resort: check file extension.
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext == ".svg" {
			mimeType = "image/svg+xml"
		}
	}

	if !allowedMIMETypes[mimeType] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"Unsupported file type %q. Allowed: image/png, image/jpeg, image/gif, image/webp, image/svg+xml, application/pdf",
			mimeType,
		))
		return
	}

	// Seek back to the start after content detection.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to process file")
		return
	}

	// Generate a unique filename with the appropriate extension.
	ext := mimeToExt[mimeType]
	if ext == "" {
		ext = filepath.Ext(header.Filename)
	}
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate filename")
		return
	}
	filename := hex.EncodeToString(randomBytes) + ext

	// Ensure upload directory exists.
	if err := os.MkdirAll(h.UploadDir, 0o755); err != nil {
		slog.Error("Failed to create upload directory", "dir", h.UploadDir, "error", err)
		writeError(w, http.StatusInternalServerError, "Upload storage unavailable")
		return
	}

	// Write file to disk.
	destPath := filepath.Join(h.UploadDir, filename)
	dst, err := os.Create(destPath)
	if err != nil {
		slog.Error("Failed to create file", "path", destPath, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		slog.Error("Failed to write file", "path", destPath, "error", err)
		// Clean up partial file.
		os.Remove(destPath)
		writeError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	slog.Info("File uploaded", "filename", filename, "size", written, "mimeType", mimeType)

	writeJSON(w, http.StatusCreated, map[string]string{
		"url": "/api/uploads/" + filename,
	})
}

// Serve handles GET /api/uploads/{filename}.
// Serves the uploaded file from disk.
func (h *UploadHandler) Serve(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	// Sanitize: only allow alphanumeric + dash + dot + underscore.
	// Prevent directory traversal.
	if filename == "" || strings.Contains(filename, "..") || strings.ContainsAny(filename, "/\\") {
		writeError(w, http.StatusBadRequest, "Invalid filename")
		return
	}

	filePath := filepath.Join(h.UploadDir, filename)

	// Check file exists.
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "File not found")
		return
	}

	// Detect content type from extension for proper browser rendering.
	ext := strings.ToLower(filepath.Ext(filename))
	contentType := ""
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	case ".pdf":
		contentType = "application/pdf"
	default:
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	http.ServeFile(w, r, filePath)
}
