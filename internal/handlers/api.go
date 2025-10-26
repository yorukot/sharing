package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yorukot/sharing/internal/services"
	"github.com/yorukot/sharing/internal/storage"
)

// APIHandler handles API requests
type APIHandler struct {
	fileService *services.FileService
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(storageBackend storage.Storage) *APIHandler {
	return &APIHandler{
		fileService: services.NewFileService(storageBackend),
	}
}

// UploadRequest represents the upload request payload
type UploadRequest struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Password  *string    `json:"password,omitempty"`
}

// UpdateRequest represents the update request payload
type UpdateRequest struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Password  *string    `json:"password,omitempty"`
	Slug      *string    `json:"slug,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// UploadFile handles file upload
func (h *APIHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (32 MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get file from form
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		respondError(w, "File is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Parse optional parameters
	var expiresAt *time.Time
	if expiresAtStr := r.FormValue("expires_at"); expiresAtStr != "" {
		t, err := time.Parse(time.RFC3339, expiresAtStr)
		if err != nil {
			respondError(w, "Invalid expires_at format (use RFC3339)", http.StatusBadRequest)
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
	savedFile, err := h.fileService.SaveFile(fileHeader, expiresAt, password, slug)
	if err != nil {
		if errors.Is(err, services.ErrSlugTaken) {
			respondError(w, "Slug already taken", http.StatusConflict)
			return
		}
		if errors.Is(err, services.ErrInvalidSlug) {
			respondError(w, "Invalid slug format (use lowercase letters, numbers, and hyphens only)", http.StatusBadRequest)
			return
		}
		respondError(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, savedFile, http.StatusCreated)
}

// ListFiles handles listing all files
func (h *APIHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	files, err := h.fileService.ListFiles()
	if err != nil {
		respondError(w, "Failed to list files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, files, http.StatusOK)
}

// GetFile handles getting a single file's metadata
func (h *APIHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromURL(r)
	if err != nil {
		respondError(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, err := h.fileService.GetFile(id)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			respondError(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrFileExpired) {
			respondError(w, "File has expired", http.StatusGone)
			return
		}
		respondError(w, "Failed to get file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, file, http.StatusOK)
}

// UpdateFile handles updating file metadata
func (h *APIHandler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromURL(r)
	if err != nil {
		respondError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	file, err := h.fileService.UpdateFile(id, req.ExpiresAt, req.Password, req.Slug)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			respondError(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrFileExpired) {
			respondError(w, "File has expired", http.StatusGone)
			return
		}
		if errors.Is(err, services.ErrSlugTaken) {
			respondError(w, "Slug already taken", http.StatusConflict)
			return
		}
		if errors.Is(err, services.ErrInvalidSlug) {
			respondError(w, "Invalid slug format (use lowercase letters, numbers, and hyphens only)", http.StatusBadRequest)
			return
		}
		respondError(w, "Failed to update file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, file, http.StatusOK)
}

// DeleteFile handles file deletion
func (h *APIHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromURL(r)
	if err != nil {
		respondError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.fileService.DeleteFile(id); err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			respondError(w, "File not found", http.StatusNotFound)
			return
		}
		respondError(w, "Failed to delete file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DownloadFile handles file download with password validation
func (h *APIHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromURL(r)
	if err != nil {
		respondError(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, err := h.fileService.GetFile(id)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			respondError(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrFileExpired) {
			respondError(w, "File has expired", http.StatusGone)
			return
		}
		respondError(w, "Failed to get file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate password if required
	password := r.URL.Query().Get("password")
	if err := h.fileService.ValidatePassword(file, password); err != nil {
		if errors.Is(err, services.ErrPasswordRequired) {
			respondError(w, "Password required", http.StatusUnauthorized)
			return
		}
		if errors.Is(err, services.ErrInvalidPassword) {
			respondError(w, "Invalid password", http.StatusForbidden)
			return
		}
		respondError(w, "Password validation failed", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.OriginalName+"\"")
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(file.FileSize, 10))

	// Get file reader from storage
	reader, err := h.fileService.GetFileReader(file)
	if err != nil {
		respondError(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Copy file content to response
	if _, err := io.Copy(w, reader); err != nil {
		// Log error but don't send response as headers already sent
		return
	}
}

// Helper functions

func getIDFromURL(r *http.Request) (uint, error) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		return 0, errors.New("ID parameter is required")
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, errors.New("invalid ID format")
	}

	return uint(id), nil
}

func respondJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, message string, status int) {
	respondJSON(w, ErrorResponse{Error: message}, status)
}
