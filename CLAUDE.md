# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

```bash
# Setup
make setup              # Create .env from .env.example and directories
go mod download         # Install dependencies

# Development
make run                # Run the application directly
make dev                # Run with auto-reload (requires air)
go run main.go          # Alternative: run directly

# Building
make build              # Build binary: ./sharing
go build -o sharing main.go

# Testing
make test               # Run all tests
go test -v ./...        # Run tests with verbose output

# Code Quality
make fmt                # Format code
make lint               # Run linter (requires golangci-lint)

# Docker
make docker-build       # Build Docker image
make docker-run         # Run in container with volume mount

# Cleanup
make clean              # Remove binary and data directory
```

## Architecture

This is a file sharing service built with **clean architecture** principles using Go, Chi router, GORM, and SQLite.

### Four-Layer Architecture

**Layer 0: Storage** (`internal/storage/`)
- Storage interface for pluggable backends
- `LocalStorage`: Filesystem-based storage (default)
- `S3Storage`: S3-compatible object storage (AWS S3, MinIO, Cloudflare R2, etc.)
- Methods: `Save()`, `Get()`, `Delete()`, `Exists()`

**Layer 1: Models** (`internal/models/`)
- `File` model with slug support, password hashing, expiration
- Business methods: `IsExpired()`, `HasPassword()`
- GORM soft deletes enabled

**Layer 2: Services** (`internal/services/`)
- `FileService` encapsulates all business logic
- Handles: slug generation/validation, password bcrypt hashing, file I/O, expiration cleanup
- Storage-agnostic: uses injected `Storage` interface
- Returns semantic errors: `ErrFileNotFound`, `ErrSlugTaken`, `ErrInvalidPassword`

**Layer 3: Handlers** (`internal/handlers/`)
- **API handlers** (`api.go`): RESTful JSON endpoints with API key authentication
- **Web handlers** (`web.go`): HTMX-based UI with API key authentication
- **Public handlers** (`public.go`): No authentication, slug-based sharing with optional password prompts

### Request Flow

```
HTTP Request
  ↓
Middleware (API key check for /api and /web routes)
  ↓
Handler (parse request, validate input)
  ↓
Service (business logic, database operations)
  ↓
Storage Interface (LocalStorage or S3Storage)
  ↓
Database/Filesystem/S3
```

## Authentication Model

**Single API Key System** (from environment variable `API_KEY`)

- **Protected Routes** (`/api/*`, `/web/*` except `/web/` index):
  - API routes require `X-API-Key` header
  - Web routes require API key via login form (stored in session)
  - Middleware: `internal/middleware/auth.go`

- **Public Routes** (`/{slug}`, `/d/{slug}`):
  - No API key required
  - Optional password protection per file (validated at download time)
  - Anyone with the link can access

**Key Implementation Details:**
- API key validation happens in middleware layer before reaching handlers
- Public handlers (`internal/handlers/public.go`) are NOT wrapped with auth middleware
- Password protection is file-level, not route-level (bcrypt comparison in `FileService.ValidatePassword()`)

## Slug System

Slugs are URL-safe short links (e.g., `example.com/my-document`)

**Validation Rules** (`internal/services/file.go:validateSlug()`):
- Regex: `^[a-z0-9-]+$` (lowercase alphanumeric + hyphens only)
- Length: 1-100 characters
- Must be unique across all files (database unique index)

**Auto-Generation** (`generateSlugFromFilename()`):
1. Remove file extension
2. Convert to lowercase
3. Replace spaces/underscores with hyphens
4. Strip non-alphanumeric characters (except hyphens)
5. Collapse consecutive hyphens
6. If slug is taken, append random hex suffix (e.g., `document-a3b2`)
7. If filename is invalid, use `file-{random}` pattern

**Custom Slugs:**
- Users can provide custom slugs via API (`slug` form field) or Web UI
- Service layer validates format and uniqueness before accepting
- Returns `ErrSlugTaken` or `ErrInvalidSlug` if invalid

## Password Protection Flow

**Upload with Password:**
1. User provides optional `password` field during upload
2. Service hashes with `bcrypt.GenerateFromPassword()` (default cost: 10)
3. Hash stored in `File.PasswordHash` (nullable field)

**Download with Password:**
1. Public user visits `/{slug}` → sees password prompt if `File.HasPassword()` returns true
2. User submits password via form
3. Handler calls `FileService.ValidatePassword()` → `bcrypt.CompareHashAndPassword()`
4. On success: file downloads; on failure: error message

**Password Updates:**
- PATCH `/api/files/{id}` with `password` field
- Empty string removes password protection (sets `password_hash` to NULL)
- Non-empty string generates new bcrypt hash

## File Lifecycle

**1. Upload** (`handlers/api.go:UploadFile()` or `handlers/web.go:UploadFileWeb()`):
- Multipart form parsing (32 MB limit)
- Generate unique filename with original extension (random hex + ext)
- Save to filesystem (`DATA_DIR`)
- Create database record with metadata

**2. Storage:**
- Files stored with unique hex IDs in `DATA_DIR` (e.g., `a1b2c3d4e5f6...pdf`)
- Database stores: original filename, size, content type, slug, password hash, expiration
- GORM tracks: `created_at`, `updated_at`, `deleted_at` (soft delete)

**3. Expiration:**
- Optional `expires_at` timestamp (nullable)
- Checked on every file access via `File.IsExpired()` method
- Returns `ErrFileExpired` if past expiration time

**4. Cleanup** (`main.go:startCleanupJob()`):
- Background goroutine runs every 1 hour
- Queries for files where `expires_at <= NOW()`
- Deletes from filesystem and database (soft delete)
- Runs immediately on startup + periodically

**5. Deletion:**
- Manual deletion via API/Web UI
- Removes file from disk with `os.Remove()`
- Soft deletes from database (GORM sets `deleted_at`)

## Environment Configuration

Required variables in `.env` (see `.env.example`):

```bash
# Authentication (REQUIRED)
API_KEY=your-secret-api-key-here  # Single API key for all auth

# Server
PORT=8080                          # Server port (default: 8080)
DB_PATH=./data/sharing.db          # SQLite database path

# Storage Backend (default: local)
STORAGE_TYPE=local                 # "local" or "s3"

# Local Storage (when STORAGE_TYPE=local)
DATA_DIR=./data                    # File storage directory

# S3 Storage (when STORAGE_TYPE=s3)
S3_BUCKET=your-bucket-name         # REQUIRED for S3
S3_REGION=us-east-1                # AWS region (default: us-east-1)
S3_ACCESS_KEY_ID=your-key          # REQUIRED for S3
S3_SECRET_ACCESS_KEY=your-secret   # REQUIRED for S3
S3_ENDPOINT=                       # Optional: custom endpoint (MinIO, R2, etc.)
S3_USE_PATH_STYLE=false            # Optional: path-style URLs (needed for MinIO)
```

**Critical:**
- `API_KEY` is the only secret; change default in production
- Database auto-migrates on startup (GORM auto-migration)
- For local storage: ensure `DATA_DIR` has write permissions
- For S3 storage: ensure bucket exists and credentials have read/write permissions

**S3-Compatible Services:**
- **AWS S3**: Use default settings, specify region
- **MinIO**: Set `S3_ENDPOINT`, `S3_USE_PATH_STYLE=true`
- **Cloudflare R2**: Set `S3_ENDPOINT` to account endpoint
- **DigitalOcean Spaces**: Set `S3_ENDPOINT` to region endpoint

## Router Structure

Routes are defined in `main.go` using Chi router:

```
/                          → Redirect to /web/
/health                    → Health check (no auth)

/api/*                     → API endpoints (API key required)
  POST   /upload           → Upload file
  GET    /files            → List all files
  GET    /files/{id}       → Get file metadata
  PATCH  /files/{id}       → Update slug/password/expiry
  DELETE /files/{id}       → Delete file
  GET    /download/{id}    → Download by ID (with password param)

/web/*                     → Web UI (API key required for management)
  GET    /                 → Index/login page (public)
  POST   /upload           → Upload via web form (protected)
  GET    /files            → List files as HTML (protected)
  GET    /edit/{id}        → Edit form (protected)
  POST   /update/{id}      → Update via form (protected)
  DELETE /files/{id}       → Delete via HTMX (protected)
  GET    /download/{id}    → Download via web (protected)

/{slug}                    → Public share page (no auth, optional password)
/d/{slug}                  → Direct download (no auth, password in query param)
```

**Important:**
- Public routes (`/{slug}`) are **catch-all** and must be defined LAST
- Middleware is applied per route group, not globally for auth
- Health check is defined before catch-all to prevent slug conflicts

## Docker Deployment

Multi-stage Dockerfile:

**Build Stage:**
- golang:1.24-alpine with CGO enabled (required for SQLite)
- Compiles static binary with `CGO_ENABLED=1`

**Runtime Stage:**
- alpine:latest with minimal dependencies
- Runs as non-root user (`appuser:1000`)
- Exposes port 8080
- Health check hits `/health` endpoint every 30s
- Volume mount `/app/data` for persistence

**Usage:**
```bash
docker build -t file-sharing .
docker run -p 8080:8080 \
  -e API_KEY=your-key \
  -v $(pwd)/data:/app/data \
  file-sharing
```

## Key Dependencies

- **chi** (v5): Lightweight HTTP router with middleware support
- **GORM** (v1.31.0): ORM with auto-migration, soft deletes
- **godotenv**: Environment variable loading from `.env`
- **bcrypt** (golang.org/x/crypto): Password hashing
- **SQLite driver** (gorm.io/driver/sqlite): Embedded database
- **AWS SDK Go v2**: S3-compatible storage client
  - `github.com/aws/aws-sdk-go-v2/aws`
  - `github.com/aws/aws-sdk-go-v2/config`
  - `github.com/aws/aws-sdk-go-v2/service/s3`
  - `github.com/aws/aws-sdk-go-v2/credentials`

## Important Patterns

**Slug Uniqueness:**
- Database has UNIQUE index on `slug` column
- Service layer validates uniqueness before INSERT
- Auto-generation retries with random suffixes if collision detected
- Max 100 attempts before giving up

**Error Handling:**
- Services return semantic errors (e.g., `ErrFileNotFound`)
- Handlers map errors to HTTP status codes using `errors.Is()`
- Example: `ErrFileNotFound` → 404, `ErrSlugTaken` → 409, `ErrInvalidPassword` → 403

**Security:**
- Passwords never stored in plaintext (always bcrypt hashed)
- API key checked in middleware (single point of enforcement)
- Files stored with random hex IDs (prevents enumeration)
- GORM soft deletes (data recovery possible)

**File Storage:**
- Storage backend abstraction via `Storage` interface
- Original filenames preserved in `File.OriginalName` for download headers
- Unique filenames prevent collisions and hide original names
- Extension preserved for correct MIME type handling
- `File.FilePath` stores either local path or S3 key depending on backend
- Seamless switching between local and S3 storage via environment variable

**Storage Implementations:**
- **LocalStorage** (`internal/storage/local.go`):
  - Stores files in `DATA_DIR` directory
  - Uses `os` package for file operations
  - `FilePath` contains full disk path (e.g., `/app/data/abc123.pdf`)
  
- **S3Storage** (`internal/storage/s3.go`):
  - Stores files in configured S3 bucket
  - Uses AWS SDK v2 for all operations
  - `FilePath` contains S3 key only (e.g., `abc123.pdf`)
  - Supports custom endpoints for S3-compatible services
  - Path-style vs virtual-hosted URLs configurable
