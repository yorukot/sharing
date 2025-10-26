package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/yorukot/sharing/internal/database"
	"github.com/yorukot/sharing/internal/handlers"
	mw "github.com/yorukot/sharing/internal/middleware"
	"github.com/yorukot/sharing/internal/services"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	// Get configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/sharing.db"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize database
	if err := database.Initialize(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Start background cleanup job
	startCleanupJob(dataDir)

	// Initialize handlers
	apiHandler := handlers.NewAPIHandler()
	webHandler := handlers.NewWebHandler()
	publicHandler := handlers.NewPublicHandler()

	// Setup router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.Compress(5))

	// API routes (protected with API key)
	r.Route("/api", func(r chi.Router) {
		r.Use(mw.APIKeyAuth)

		r.Post("/upload", apiHandler.UploadFile)
		r.Get("/files", apiHandler.ListFiles)
		r.Get("/files/{id}", apiHandler.GetFile)
		r.Patch("/files/{id}", apiHandler.UpdateFile)
		r.Delete("/files/{id}", apiHandler.DeleteFile)
		r.Get("/download/{id}", apiHandler.DownloadFile)
	})

	// Web routes (protected with API key for management)
	r.Route("/web", func(r chi.Router) {
		// Public index page (shows login if not authenticated)
		r.Get("/", webHandler.Index)

		// Protected management routes
		r.Group(func(r chi.Router) {
			r.Use(mw.APIKeyAuth)

			r.Post("/upload", webHandler.UploadFileWeb)
			r.Get("/files", webHandler.FileList)
			r.Get("/edit/{id}", webHandler.EditForm)
			r.Post("/update/{id}", webHandler.UpdateFileWeb)
			r.Delete("/files/{id}", webHandler.DeleteFileWeb)
			r.Get("/download/{id}", webHandler.DownloadFileWeb)
		})
	})

	// Health check endpoint (before catch-all routes)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Redirect root to web UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/web/", http.StatusMovedPermanently)
	})

	// Public sharing routes (no API key required)
	// Direct download route
	r.Get("/d/{slug}", publicHandler.DownloadBySlug)

	// Share page route (catch-all, must be last)
	r.Get("/{slug}", publicHandler.SharePage)

	// Start server
	log.Printf("Starting server on port %s", port)
	log.Printf("Web UI: http://localhost:%s/web/", port)
	log.Printf("API: http://localhost:%s/api/", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// startCleanupJob runs a background job to clean up expired files
func startCleanupJob(dataDir string) {
	fileService := services.NewFileService(dataDir)

	// Run cleanup every hour
	ticker := time.NewTicker(1 * time.Hour)

	go func() {
		// Run immediately on startup
		if err := fileService.CleanupExpiredFiles(); err != nil {
			log.Printf("Cleanup error: %v", err)
		} else {
			log.Println("Initial cleanup completed")
		}

		// Then run periodically
		for range ticker.C {
			if err := fileService.CleanupExpiredFiles(); err != nil {
				log.Printf("Cleanup error: %v", err)
			} else {
				log.Println("Cleanup completed")
			}
		}
	}()
}
