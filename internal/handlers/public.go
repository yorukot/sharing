package handlers

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yorukot/sharing/internal/services"
	"github.com/yorukot/sharing/internal/storage"
)

// PublicHandler handles public sharing routes (no API key required)
type PublicHandler struct {
	fileService *services.FileService
	templates   *template.Template
}

// NewPublicHandler creates a new public handler
func NewPublicHandler(storageBackend storage.Storage) *PublicHandler {
	// Parse templates for public pages
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		// If templates don't exist yet, create empty template
		tmpl = template.New("public")
	}

	return &PublicHandler{
		fileService: services.NewFileService(storageBackend),
		templates:   tmpl,
	}
}

// renderPasswordPrompt renders a unified password prompt page
func (h *PublicHandler) renderPasswordPrompt(w http.ResponseWriter, originalName, filename string, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Password Required</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
			display: flex;
			align-items: center;
			justify-content: center;
			min-height: 100vh;
			background: #f5f5f5;
		}
		.container {
			max-width: 450px;
			width: 90%;
			text-align: center;
		}
		h1 {
			font-size: 24px;
			font-weight: 600;
			color: #000;
			margin-bottom: 10px;
		}
		p {
			font-size: 14px;
			color: #666;
			margin-bottom: 30px;
		}
		input[type="password"] {
			width: 100%;
			padding: 12px 16px;
			border: 1px solid #ddd;
			border-radius: 4px;
			font-size: 14px;
			margin-bottom: 15px;
			background: white;
		}
		input[type="password"]:focus {
			outline: none;
			border-color: #3498db;
		}
		button {
			width: 100%;
			padding: 12px;
			background: #3498db;
			color: white;
			border: none;
			border-radius: 4px;
			font-size: 14px;
			font-weight: 500;
			cursor: pointer;
			transition: background 0.2s;
		}
		button:hover {
			background: #2980b9;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>Password Required</h1>
		<p>This file is password protected.</p>
		<form onsubmit="download(event)">
			<input type="password" id="pwd" placeholder="Enter password" required autofocus>
			<button type="submit">Download</button>
		</form>
	</div>
	<script>
		function download(e) {
			e.preventDefault();
			const pwd = document.getElementById('pwd').value;
			window.location.href = '/d/` + originalName + `?password=' + encodeURIComponent(pwd);
		}
	</script>
</body>
</html>`))
}

// SharePage redirects directly to download (with password prompt if needed)
func (h *PublicHandler) SharePage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	file, err := h.fileService.GetFileBySlug(slug)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrFileExpired) {
			http.Error(w, "This file has expired", http.StatusGone)
			return
		}
		http.Error(w, "Failed to load file", http.StatusInternalServerError)
		return
	}

	// If password protected, show simple password prompt
	if file.HasPassword() {
		// For password prompt, always use original filename in the /d/ URL
		h.renderPasswordPrompt(w, file.OriginalName, file.OriginalName, http.StatusOK)
		return
	}

	// No password, redirect directly to download using original filename
	// URL encode the filename to handle Unicode characters properly
	http.Redirect(w, r, "/d/"+url.PathEscape(file.OriginalName), http.StatusFound)
}

// DownloadByOriginalName handles file download via original filename (public, no API key required)
func (h *PublicHandler) DownloadByOriginalName(w http.ResponseWriter, r *http.Request) {
	encodedFilename := chi.URLParam(r, "filename")

	// URL decode the filename to handle Unicode characters
	filename, err := url.QueryUnescape(encodedFilename)
	if err != nil {
		// If decoding fails, use the original
		filename = encodedFilename
	}

	file, err := h.fileService.GetFileByOriginalName(filename)
	if err != nil {
		if errors.Is(err, services.ErrFileNotFound) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrFileExpired) {
			http.Error(w, "This file has expired", http.StatusGone)
			return
		}
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}

	// Validate password if required
	password := r.URL.Query().Get("password")
	if err := h.fileService.ValidatePassword(file, password); err != nil {
		if errors.Is(err, services.ErrPasswordRequired) {
			// Show password prompt page
			h.renderPasswordPrompt(w, file.OriginalName, file.OriginalName, http.StatusUnauthorized)
			return
		}
		if errors.Is(err, services.ErrInvalidPassword) {
			http.Error(w, "Invalid password", http.StatusForbidden)
			return
		}
		http.Error(w, "Password validation failed", http.StatusInternalServerError)
		return
	}

	// Set headers for inline viewing (browser preview instead of download)
	w.Header().Set("Content-Disposition", "inline; filename=\""+file.OriginalName+"\"")
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(file.FileSize, 10))

	// Get file reader from storage
	reader, err := h.fileService.GetFileReader(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Copy file content to response
	if _, err := io.Copy(w, reader); err != nil {
		// Log error but don't send response as headers already sent
		return
	}
}
