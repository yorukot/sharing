package handlers

import (
	"errors"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yorukot/sharing/internal/services"
)

// WebHandler handles web UI requests
type WebHandler struct {
	fileService *services.FileService
	templates   *template.Template
}

// NewWebHandler creates a new web handler
func NewWebHandler() *WebHandler {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// Parse templates
	tmpl := template.Must(template.ParseGlob("templates/*.html"))

	return &WebHandler{
		fileService: services.NewFileService(dataDir),
		templates:   tmpl,
	}
}

// Index renders the main page
func (h *WebHandler) Index(w http.ResponseWriter, r *http.Request) {
	files, err := h.fileService.ListFiles()
	if err != nil {
		http.Error(w, "Failed to load files", http.StatusInternalServerError)
		return
	}

	data := struct {
		Files interface{}
	}{
		Files: files,
	}

	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// UploadFileWeb handles file upload from web UI
func (h *WebHandler) UploadFileWeb(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (32 MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get file from form
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Parse optional parameters
	var expiresAt *time.Time
	if expiresAtStr := r.FormValue("expires_at"); expiresAtStr != "" {
		t, err := time.Parse("2006-01-02T15:04", expiresAtStr)
		if err != nil {
			http.Error(w, "Invalid expiry date format", http.StatusBadRequest)
			return
		}
		expiresAt = &t
	}

	var password *string
	if pwd := r.FormValue("password"); pwd != "" {
		password = &pwd
	}

	var slug *string
	if s := r.FormValue("slug"); s != "" {
		slug = &s
	}

	// Save file
	_, err = h.fileService.SaveFile(fileHeader, expiresAt, password, slug)
	if err != nil {
		if errors.Is(err, services.ErrSlugTaken) {
			http.Error(w, "Slug already taken", http.StatusConflict)
			return
		}
		if errors.Is(err, services.ErrInvalidSlug) {
			http.Error(w, "Invalid slug format (use lowercase letters, numbers, and hyphens only)", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return updated file list
	h.FileList(w, r)
}

// FileList returns the file list HTML fragment
func (h *WebHandler) FileList(w http.ResponseWriter, r *http.Request) {
	files, err := h.fileService.ListFiles()
	if err != nil {
		http.Error(w, "Failed to load files", http.StatusInternalServerError)
		return
	}

	data := struct {
		Files interface{}
	}{
		Files: files,
	}

	if err := h.templates.ExecuteTemplate(w, "file-list", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// DeleteFileWeb handles file deletion from web UI
func (h *WebHandler) DeleteFileWeb(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.fileService.DeleteFile(uint(id)); err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to delete file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// EditForm returns the edit form for a file
func (h *WebHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	file, err := h.fileService.GetFile(uint(id))
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	data := struct {
		File interface{}
	}{
		File: file,
	}

	if err := h.templates.ExecuteTemplate(w, "edit-form", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// UpdateFileWeb handles file update from web UI
func (h *WebHandler) UpdateFileWeb(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	var expiresAt *time.Time
	if expiresAtStr := r.FormValue("expires_at"); expiresAtStr != "" {
		t, err := time.Parse("2006-01-02T15:04", expiresAtStr)
		if err != nil {
			http.Error(w, "Invalid expiry date format", http.StatusBadRequest)
			return
		}
		expiresAt = &t
	}

	var password *string
	if pwd := r.FormValue("password"); pwd != "" {
		password = &pwd
	}

	var slug *string
	if s := r.FormValue("slug"); s != "" {
		slug = &s
	}

	file, err := h.fileService.UpdateFile(uint(id), expiresAt, password, slug)
	if err != nil {
		if errors.Is(err, services.ErrSlugTaken) {
			http.Error(w, "Slug already taken", http.StatusConflict)
			return
		}
		if errors.Is(err, services.ErrInvalidSlug) {
			http.Error(w, "Invalid slug format", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to update file", http.StatusInternalServerError)
		return
	}

	data := struct {
		File interface{}
	}{
		File: file,
	}

	if err := h.templates.ExecuteTemplate(w, "file-row", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// DownloadFileWeb handles file download from web UI
func (h *WebHandler) DownloadFileWeb(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	file, err := h.fileService.GetFile(uint(id))
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrFileExpired) {
			http.Error(w, "File has expired", http.StatusGone)
			return
		}
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}

	// Check if password is required
	if file.HasPassword() {
		password := r.URL.Query().Get("password")
		if password == "" {
			// Return password prompt
			data := struct {
				FileID uint
			}{
				FileID: file.ID,
			}
			if err := h.templates.ExecuteTemplate(w, "password-prompt", data); err != nil {
				http.Error(w, "Template error", http.StatusInternalServerError)
			}
			return
		}

		// Validate password
		if err := h.fileService.ValidatePassword(file, password); err != nil {
			http.Error(w, "Invalid password", http.StatusForbidden)
			return
		}
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.OriginalName+"\"")
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(file.FileSize, 10))

	// Serve file
	http.ServeFile(w, r, file.FilePath)
}
