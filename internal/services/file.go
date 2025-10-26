package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yorukot/sharing/internal/database"
	"github.com/yorukot/sharing/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrFileNotFound     = errors.New("file not found")
	ErrFileExpired      = errors.New("file has expired")
	ErrInvalidPassword  = errors.New("invalid password")
	ErrPasswordRequired = errors.New("password required")
	ErrSlugTaken        = errors.New("slug already taken")
	ErrInvalidSlug      = errors.New("invalid slug format")
)

var slugRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// FileService handles file operations
type FileService struct {
	dataDir string
}

// NewFileService creates a new file service instance
func NewFileService(dataDir string) *FileService {
	return &FileService{
		dataDir: dataDir,
	}
}

// SaveFile saves an uploaded file to disk and creates a database record
func (s *FileService) SaveFile(fileHeader *multipart.FileHeader, expiresAt *time.Time, password *string, slug *string) (*models.File, error) {
	// Generate unique filename
	uniqueFilename, err := s.generateUniqueFilename(fileHeader.Filename)
	if err != nil {
		return nil, fmt.Errorf("failed to generate filename: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Full path on disk
	filePath := filepath.Join(s.dataDir, uniqueFilename)

	// Open uploaded file
	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(filePath) // Clean up on error
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	// Hash password if provided
	var passwordHash *string
	if password != nil && *password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			os.Remove(filePath) // Clean up on error
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		hashStr := string(hash)
		passwordHash = &hashStr
	}

	// Generate or validate slug
	fileSlug := ""
	if slug != nil && *slug != "" {
		// User provided custom slug - validate and check uniqueness
		if err := s.validateSlug(*slug); err != nil {
			os.Remove(filePath) // Clean up on error
			return nil, err
		}
		if err := s.checkSlugUnique(*slug); err != nil {
			os.Remove(filePath) // Clean up on error
			return nil, err
		}
		fileSlug = *slug
	} else {
		// Auto-generate slug from filename
		fileSlug, err = s.generateSlugFromFilename(fileHeader.Filename)
		if err != nil {
			os.Remove(filePath) // Clean up on error
			return nil, fmt.Errorf("failed to generate slug: %w", err)
		}
	}

	// Create database record
	file := &models.File{
		Filename:     uniqueFilename,
		OriginalName: fileHeader.Filename,
		FilePath:     filePath,
		FileSize:     fileHeader.Size,
		ContentType:  fileHeader.Header.Get("Content-Type"),
		Slug:         fileSlug,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
	}

	if err := database.DB.Create(file).Error; err != nil {
		os.Remove(filePath) // Clean up on error
		return nil, fmt.Errorf("failed to create database record: %w", err)
	}

	return file, nil
}

// GetFile retrieves a file by ID
func (s *FileService) GetFile(id uint) (*models.File, error) {
	var file models.File
	if err := database.DB.First(&file, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	if file.IsExpired() {
		return nil, ErrFileExpired
	}

	return &file, nil
}

// GetFileBySlug retrieves a file by its slug
func (s *FileService) GetFileBySlug(slug string) (*models.File, error) {
	var file models.File
	if err := database.DB.Where("slug = ?", slug).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	if file.IsExpired() {
		return nil, ErrFileExpired
	}

	return &file, nil
}

// ListFiles retrieves all non-expired files
func (s *FileService) ListFiles() ([]models.File, error) {
	var files []models.File
	if err := database.DB.Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("created_at DESC").
		Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// UpdateFile updates a file's expiry date, password, and/or slug
func (s *FileService) UpdateFile(id uint, expiresAt *time.Time, password *string, slug *string) (*models.File, error) {
	file, err := s.GetFile(id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	// Update expiry date
	if expiresAt != nil {
		updates["expires_at"] = expiresAt
	}

	// Update password
	if password != nil {
		if *password == "" {
			// Remove password protection
			updates["password_hash"] = nil
		} else {
			// Set new password
			hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
			if err != nil {
				return nil, fmt.Errorf("failed to hash password: %w", err)
			}
			updates["password_hash"] = string(hash)
		}
	}

	// Update slug
	if slug != nil && *slug != "" && *slug != file.Slug {
		// Validate new slug
		if err := s.validateSlug(*slug); err != nil {
			return nil, err
		}
		// Check if slug is unique (excluding current file)
		var count int64
		database.DB.Model(&models.File{}).Where("slug = ? AND id != ?", *slug, id).Count(&count)
		if count > 0 {
			return nil, ErrSlugTaken
		}
		updates["slug"] = *slug
	}

	if err := database.DB.Model(file).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update file: %w", err)
	}

	// Reload to get updated values
	return s.GetFile(id)
}

// DeleteFile deletes a file from disk and database
func (s *FileService) DeleteFile(id uint) error {
	file, err := s.GetFile(id)
	if err != nil {
		return err
	}

	// Delete file from disk
	if err := os.Remove(file.FilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file from disk: %w", err)
	}

	// Delete from database (soft delete)
	if err := database.DB.Delete(file).Error; err != nil {
		return fmt.Errorf("failed to delete from database: %w", err)
	}

	return nil
}

// ValidatePassword checks if the provided password matches the file's password hash
func (s *FileService) ValidatePassword(file *models.File, password string) error {
	if !file.HasPassword() {
		return nil // No password protection
	}

	if password == "" {
		return ErrPasswordRequired
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*file.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidPassword
	}

	return nil
}

// CleanupExpiredFiles removes expired files from disk and database
func (s *FileService) CleanupExpiredFiles() error {
	var expiredFiles []models.File
	if err := database.DB.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Find(&expiredFiles).Error; err != nil {
		return err
	}

	for _, file := range expiredFiles {
		// Delete file from disk
		if err := os.Remove(file.FilePath); err != nil && !os.IsNotExist(err) {
			// Log error but continue
			fmt.Printf("Warning: failed to delete expired file %s: %v\n", file.FilePath, err)
		}

		// Delete from database
		if err := database.DB.Delete(&file).Error; err != nil {
			fmt.Printf("Warning: failed to delete expired file record %d: %v\n", file.ID, err)
		}
	}

	return nil
}

// generateUniqueFilename creates a unique filename with the original extension
func (s *FileService) generateUniqueFilename(originalName string) (string, error) {
	// Generate random bytes
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// Convert to hex string
	uniqueID := hex.EncodeToString(randomBytes)

	// Preserve original extension
	ext := filepath.Ext(originalName)

	return uniqueID + ext, nil
}

// validateSlug checks if a slug is in valid format
func (s *FileService) validateSlug(slug string) error {
	if len(slug) < 1 || len(slug) > 100 {
		return ErrInvalidSlug
	}
	if !slugRegex.MatchString(slug) {
		return ErrInvalidSlug
	}
	return nil
}

// checkSlugUnique checks if a slug is already taken
func (s *FileService) checkSlugUnique(slug string) error {
	var count int64
	database.DB.Model(&models.File{}).Where("slug = ?", slug).Count(&count)
	if count > 0 {
		return ErrSlugTaken
	}
	return nil
}

// generateSlugFromFilename creates a URL-safe slug from a filename
func (s *FileService) generateSlugFromFilename(filename string) (string, error) {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Remove any non-alphanumeric characters except hyphens
	slug = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(slug, "")

	// Remove consecutive hyphens
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")

	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")

	// If slug is empty or invalid, generate random
	if slug == "" || len(slug) < 3 {
		randomBytes := make([]byte, 4)
		rand.Read(randomBytes)
		slug = "file-" + hex.EncodeToString(randomBytes)
	}

	// Make unique by appending random suffix if taken
	originalSlug := slug
	for i := 0; i < 100; i++ {
		if err := s.checkSlugUnique(slug); err == nil {
			return slug, nil
		}

		// Append random suffix
		randomBytes := make([]byte, 2)
		rand.Read(randomBytes)
		slug = originalSlug + "-" + hex.EncodeToString(randomBytes)
	}

	return "", fmt.Errorf("failed to generate unique slug")
}
