package handlers

import (
	"errors"
	"html/template"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/yorukot/sharing/internal/services"
)

// PublicHandler handles public sharing routes (no API key required)
type PublicHandler struct {
	fileService *services.FileService
	templates   *template.Template
}

// NewPublicHandler creates a new public handler
func NewPublicHandler() *PublicHandler {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// Parse templates for public pages
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		// If templates don't exist yet, create empty template
		tmpl = template.New("public")
	}

	return &PublicHandler{
		fileService: services.NewFileService(dataDir),
		templates:   tmpl,
	}
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Password Required - ` + file.OriginalName + `</title>
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
			display: flex;
			align-items: center;
			justify-content: center;
			min-height: 100vh;
			margin: 0;
			background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
		}
		.box {
			background: white;
			padding: 40px;
			border-radius: 12px;
			box-shadow: 0 10px 40px rgba(0,0,0,0.2);
			max-width: 400px;
			width: 90%;
			text-align: center;
		}
		h2 { color: #2c3e50; margin-bottom: 10px; }
		p { color: #7f8c8d; margin-bottom: 20px; }
		.filename {
			background: #f8f9fa;
			padding: 10px;
			border-radius: 6px;
			margin-bottom: 20px;
			word-break: break-word;
			font-family: monospace;
			font-size: 13px;
		}
		input {
			width: 100%;
			padding: 12px;
			border: 1px solid #ddd;
			border-radius: 6px;
			font-size: 14px;
			margin-bottom: 15px;
			box-sizing: border-box;
		}
		button {
			width: 100%;
			padding: 12px;
			background: #3498db;
			color: white;
			border: none;
			border-radius: 6px;
			font-size: 14px;
			font-weight: 500;
			cursor: pointer;
			transition: background 0.3s;
		}
		button:hover { background: #2980b9; }
	</style>
</head>
<body>
	<div class="box">
		<h2>🔒 Password Required</h2>
		<p>This file is password protected</p>
		<div class="filename">` + file.OriginalName + `</div>
		<form onsubmit="download(event)">
			<input type="password" id="pwd" placeholder="Enter password" required autofocus>
			<button type="submit">Download File</button>
		</form>
	</div>
	<script>
		function download(e) {
			e.preventDefault();
			const pwd = document.getElementById('pwd').value;
			window.location.href = '/d/` + slug + `?password=' + encodeURIComponent(pwd);
		}
	</script>
</body>
</html>`))
		return
	}

	// No password, redirect directly to download
	http.Redirect(w, r, "/d/"+slug, http.StatusFound)
}

// DownloadBySlug handles file download via slug (public, no API key required)
func (h *PublicHandler) DownloadBySlug(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}

	// Validate password if required
	password := r.URL.Query().Get("password")
	if err := h.fileService.ValidatePassword(file, password); err != nil {
		if errors.Is(err, services.ErrPasswordRequired) {
			// Show password prompt page
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>Password Required</title>
	<style>
		body { font-family: sans-serif; max-width: 400px; margin: 100px auto; padding: 20px; }
		input { width: 100%; padding: 10px; margin: 10px 0; border: 1px solid #ddd; border-radius: 4px; }
		button { width: 100%; padding: 10px; background: #3498db; color: white; border: none; border-radius: 4px; cursor: pointer; }
		button:hover { background: #2980b9; }
	</style>
</head>
<body>
	<h2>Password Required</h2>
	<p>This file is password protected.</p>
	<form method="GET">
		<input type="password" name="password" placeholder="Enter password" required autofocus>
		<button type="submit">Download</button>
	</form>
</body>
</html>`))
			return
		}
		if errors.Is(err, services.ErrInvalidPassword) {
			http.Error(w, "Invalid password", http.StatusForbidden)
			return
		}
		http.Error(w, "Password validation failed", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.OriginalName+"\"")
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(file.FileSize, 10))

	// Serve file
	http.ServeFile(w, r, file.FilePath)
}
