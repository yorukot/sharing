package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/yorukot/sharing/internal/database"
	"github.com/yorukot/sharing/internal/handlers"
	mw "github.com/yorukot/sharing/internal/middleware"
	"github.com/yorukot/sharing/internal/services"
	"github.com/yorukot/sharing/internal/storage"
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

	// Initialize storage backend
	storageBackend, err := initializeStorage()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Initialize database
	if err := database.Initialize(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize file service with storage backend
	fileService := services.NewFileService(storageBackend)

	// Start background cleanup job
	startCleanupJob(fileService)

	// Initialize handlers
	apiHandler := handlers.NewAPIHandler(storageBackend)
	webHandler := handlers.NewWebHandler(storageBackend)
	publicHandler := handlers.NewPublicHandler(storageBackend)

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
	// Direct download route by original filename
	r.Get("/d/{filename}", publicHandler.DownloadByOriginalName)

	// Share page route by slug (catch-all, must be last)
	r.Get("/{slug}", publicHandler.SharePage)

	// Start server
	log.Printf("Starting server on port %s", port)
	log.Printf("Web UI: http://localhost:%s/web/", port)
	log.Printf("API: http://localhost:%s/api/", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// initializeStorage creates and configures the storage backend based on environment variables
func initializeStorage() (storage.Storage, error) {
	storageType := strings.ToLower(os.Getenv("STORAGE_TYPE"))
	if storageType == "" {
		storageType = "local" // Default to local storage
	}

	switch storageType {
	case "local":
		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		log.Printf("Using local storage: %s", dataDir)
		return storage.NewLocalStorage(dataDir)

	case "s3":
		endpoint := os.Getenv("S3_ENDPOINT")
		bucket := os.Getenv("S3_BUCKET")
		region := os.Getenv("S3_REGION")
		accessKeyID := os.Getenv("S3_ACCESS_KEY_ID")
		secretAccessKey := os.Getenv("S3_SECRET_ACCESS_KEY")
		usePathStyleStr := os.Getenv("S3_USE_PATH_STYLE")

		// Validate required S3 configuration
		if bucket == "" {
			return nil, fmt.Errorf("S3_BUCKET is required when using S3 storage")
		}
		if region == "" {
			region = "us-east-1" // Default region
		}
		if accessKeyID == "" || secretAccessKey == "" {
			return nil, fmt.Errorf("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are required when using S3 storage")
		}

		usePathStyle := false
		if usePathStyleStr != "" {
			var err error
			usePathStyle, err = strconv.ParseBool(usePathStyleStr)
			if err != nil {
				log.Printf("Warning: invalid S3_USE_PATH_STYLE value, using default (false)")
			}
		}

		config := storage.S3Config{
			Endpoint:        endpoint,
			Bucket:          bucket,
			Region:          region,
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			UsePathStyle:    usePathStyle,
		}

		log.Printf("Using S3 storage: bucket=%s, region=%s, endpoint=%s", bucket, region, endpoint)
		return storage.NewS3Storage(config)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s (supported: local, s3)", storageType)
	}
}

// startCleanupJob runs a background job to clean up expired files
func startCleanupJob(fileService *services.FileService) {
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
