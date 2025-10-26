# File Sharing Service

A clean, simple, and secure file sharing service built with Go, Chi router, HTMX, and SQLite. Features include password protection, expiration dates, custom short links, and both API and web UI interfaces.

## Features

- **🔗 Short Links**: Custom slugs for easy sharing (e.g., `example.com/my-document`)
- **🔒 Password Protection**: Optional password protection for sensitive files
- **⏰ Expiration Dates**: Set automatic expiration dates for shared files
- **🔑 API-First Design**: RESTful API with API key authentication
- **🖥️ Web UI**: Modern, responsive HTMX-based interface with API key login
- **💾 SQLite Database**: Lightweight ORM with GORM
- **🧹 Auto Cleanup**: Background job automatically removes expired files
- **📊 File Metadata**: Track original filenames, sizes, upload dates, and more

## Quick Start

### 1. Clone and Setup

```bash
cd sharing
cp .env.example .env
```

### 2. Configure Environment

Edit `.env` and set your API key:

```env
API_KEY=your-secret-api-key-here
PORT=8080
DB_PATH=./data/sharing.db
DATA_DIR=./data
```

### 3. Install Dependencies

```bash
go mod download
```

### 4. Run the Server

```bash
go run main.go
```

The server will start on `http://localhost:8080`

- Web UI: `http://localhost:8080/web/` (requires API key login)
- API: `http://localhost:8080/api/` (requires API key header)
- Public Sharing: `http://localhost:8080/{slug}` (no authentication)

## Usage

### Web UI

1. Visit `http://localhost:8080/web/`
2. Enter your API key from `.env` file
3. Upload files with optional:
   - Custom short link (slug)
   - Expiration date
   - Password protection
4. Copy share links and send to recipients
5. Manage files: edit settings, delete, view all uploads

### Public File Sharing

When you upload a file, you get a short link like:
- `http://localhost:8080/my-document`

Recipients can:
- Visit the link directly → **Immediately downloads** (if no password)
- If password protected → **Simple password prompt**, then downloads

**No fancy landing pages** - just direct downloads!

## API Documentation

All API endpoints require the `X-API-Key` header.

### Upload File

```bash
POST /api/upload
Content-Type: multipart/form-data
X-API-Key: your-api-key

Form fields:
- file: (required) The file to upload
- slug: (optional) Custom short link (e.g., "my-document")
- expires_at: (optional) ISO 8601 datetime (RFC3339)
- password: (optional) Password protection
```

Example:
```bash
curl -X POST http://localhost:8080/api/upload \
  -H "X-API-Key: your-api-key" \
  -F "file=@document.pdf" \
  -F "slug=my-document" \
  -F "expires_at=2025-12-31T23:59:59Z" \
  -F "password=secret123"
```

Response:
```json
{
  "id": 1,
  "filename": "a1b2c3d4...pdf",
  "original_name": "document.pdf",
  "file_size": 102400,
  "content_type": "application/pdf",
  "slug": "my-document",
  "expires_at": "2025-12-31T23:59:59Z",
  "created_at": "2025-10-26T10:30:00Z",
  "updated_at": "2025-10-26T10:30:00Z"
}
```

**Share link:** `http://localhost:8080/my-document`

### List Files

```bash
GET /api/files
X-API-Key: your-api-key
```

Example:
```bash
curl http://localhost:8080/api/files \
  -H "X-API-Key: your-api-key"
```

### Get File Info

```bash
GET /api/files/{id}
X-API-Key: your-api-key
```

### Update File

Update slug, expiration date, or password:

```bash
PATCH /api/files/{id}
Content-Type: application/json
X-API-Key: your-api-key

{
  "slug": "new-custom-link",
  "expires_at": "2025-12-31T23:59:59Z",
  "password": "new-password"
}
```

Example:
```bash
curl -X PATCH http://localhost:8080/api/files/1 \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "updated-link",
    "expires_at": "2025-12-31T23:59:59Z"
  }'
```

### Delete File

```bash
DELETE /api/files/{id}
X-API-Key: your-api-key
```

Example:
```bash
curl -X DELETE http://localhost:8080/api/files/1 \
  -H "X-API-Key: your-api-key"
```

### Download File (via API)

```bash
GET /api/download/{id}?password=secret123
X-API-Key: your-api-key
```

Example:
```bash
curl -O http://localhost:8080/api/download/1?password=secret123 \
  -H "X-API-Key: your-api-key"
```

## Public Sharing Routes (No API Key Required)

These routes are for end users who receive share links:

### Share Link (Direct Download)

```
GET /{slug}
```

- **No password:** Immediately downloads file
- **With password:** Shows password prompt page, then downloads

Examples:
- `http://localhost:8080/my-document`
- `http://localhost:8080/vacation-photos`
- `http://localhost:8080/meeting-notes-2024`

### Direct Download Link

```
GET /d/{slug}?password=optional
```

Direct download URL (can include password in query string):
- `http://localhost:8080/d/my-document`
- `http://localhost:8080/d/my-document?password=secret123`

## Slug Format

Slugs must follow these rules:
- **Lowercase letters** (a-z)
- **Numbers** (0-9)
- **Hyphens** (-)
- Length: 1-100 characters

✅ Valid: `my-document`, `report-2024`, `vacation-photos`  
❌ Invalid: `My_Document`, `report@2024`, `файл`

If you don't provide a slug, one will be auto-generated from the filename.

## Project Structure

```
sharing/
├── main.go                      # Application entry point
├── go.mod                       # Go module dependencies
├── .env                         # Environment configuration
├── internal/
│   ├── models/
│   │   └── file.go             # File data model with slug field
│   ├── database/
│   │   └── db.go               # Database initialization
│   ├── middleware/
│   │   └── auth.go             # API key authentication
│   ├── handlers/
│   │   ├── api.go              # REST API handlers
│   │   ├── web.go              # Web UI handlers (protected)
│   │   └── public.go           # Public sharing handlers (no auth)
│   └── services/
│       └── file.go             # Business logic with slug generation
├── templates/
│   └── index.html              # Web UI with API key login
└── data/                       # File storage & SQLite DB
    ├── sharing.db              # SQLite database
    └── [uploaded files]        # Uploaded files with unique names
```

## Architecture

### Clean Architecture Principles

1. **Models** (`internal/models`): Data structures with slug support
2. **Database** (`internal/database`): Database connection and migrations
3. **Services** (`internal/services`): Business logic with slug validation
4. **Handlers** (`internal/handlers`): 
   - API handlers (protected)
   - Web handlers (protected)
   - Public handlers (no auth)
5. **Middleware** (`internal/middleware`): API key authentication

### Key Design Decisions

- **API-First**: API endpoints are primary, web UI is secondary
- **No User System**: Single API key from environment variable
- **File Storage**: Simple filesystem storage in `/data` directory
- **Short Links**: Custom slugs for user-friendly URLs
- **Security**: API key for management, public access for sharing
- **Direct Downloads**: No fancy landing pages, just download prompts

## Security Features

- **API Key Authentication**: All management operations require valid API key
  - Web UI login with API key
  - API requests need `X-API-Key` header
- **Password Hashing**: bcrypt for secure password storage
- **Automatic Cleanup**: Expired files removed hourly
- **Unique Filenames**: Random hex IDs prevent filename collisions
- **Slug Validation**: Prevents injection and ensures URL safety
- **Soft Deletes**: GORM soft delete for data recovery

## Configuration

Environment variables (`.env`):

| Variable | Description | Default |
|----------|-------------|---------|
| `API_KEY` | API authentication key | (required) |
| `PORT` | Server port | `8080` |
| `DB_PATH` | SQLite database path | `./data/sharing.db` |
| `DATA_DIR` | File storage directory | `./data` |

## Development

### Build

```bash
go build -o sharing main.go
```

### Run

```bash
./sharing
```

Or use the Makefile:

```bash
make build
make run
```

### Database Migrations

GORM auto-migration runs on startup. The database schema is automatically created/updated.

## Production Deployment

1. **Set a strong `API_KEY`** in production
2. Use environment variables instead of `.env` file
3. Place behind a reverse proxy (nginx, Caddy)
4. Configure SSL/TLS for HTTPS
5. Set appropriate file upload limits
6. Regular backups of `/data` directory
7. Monitor disk space for uploaded files

Example nginx config:
```nginx
server {
    listen 80;
    server_name share.example.com;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        client_max_body_size 100M;
    }
}
```

## Examples

### Upload and Share Workflow

```bash
# 1. Upload file with custom slug
curl -X POST http://localhost:8080/api/upload \
  -H "X-API-Key: demo-api-key-change-in-production" \
  -F "file=@presentation.pdf" \
  -F "slug=q4-presentation" \
  -F "expires_at=2025-12-31T23:59:59Z" \
  -F "password=team2024"

# 2. Share the link with your team
echo "Download: http://localhost:8080/q4-presentation"

# 3. Recipients visit the link
# - See password prompt
# - Enter "team2024"
# - File downloads immediately
```

### Update Share Link

```bash
# Change the slug to something more memorable
curl -X PATCH http://localhost:8080/api/files/1 \
  -H "X-API-Key: demo-api-key-change-in-production" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "best-presentation-ever"
  }'

# New share link: http://localhost:8080/best-presentation-ever
```

## Dependencies

- [Chi](https://github.com/go-chi/chi) - Lightweight HTTP router
- [GORM](https://gorm.io/) - ORM library
- [godotenv](https://github.com/joho/godotenv) - Environment variable loader
- [bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt) - Password hashing
- [HTMX](https://htmx.org/) - Frontend interactivity (CDN)

## License

MIT License

## Contributing

Contributions are welcome! Please:

1. Keep code clean and well-commented
2. Follow Go conventions and best practices
3. Test thoroughly before submitting
4. Update documentation for new features
