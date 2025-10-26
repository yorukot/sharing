package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yorukot/sharing/internal/database"
	"github.com/yorukot/sharing/internal/models"
	"github.com/yorukot/sharing/internal/storage"
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

var slugRegex = regexp.MustCompile(`^[a-zA-Z0-9\p{L}\p{N}._-]+$`)

// FileService handles file operations
type FileService struct {
	storage storage.Storage
}

// NewFileService creates a new file service instance
func NewFileService(storageBackend storage.Storage) *FileService {
	return &FileService{
		storage: storageBackend,
	}
}

// SaveFile saves an uploaded file to storage and creates a database record
func (s *FileService) SaveFile(fileHeader *multipart.FileHeader, expiresAt *time.Time, password *string, slug *string) (*models.File, error) {
	// Generate unique filename
	uniqueFilename, err := s.generateUniqueFilename(fileHeader.Filename)
	if err != nil {
		return nil, fmt.Errorf("failed to generate filename: %w", err)
	}

	// Open uploaded file
	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Save to storage backend
	storagePath, err := s.storage.Save(src, uniqueFilename, fileHeader.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to save file to storage: %w", err)
	}

	// Hash password if provided
	var passwordHash *string
	if password != nil && *password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			s.storage.Delete(storagePath) // Clean up on error
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		hashStr := string(hash)
		passwordHash = &hashStr
	}

	// Generate or validate slug
	var fileSlug string
	var uniqueOriginalName string

	if slug != nil && *slug != "" {
		// User provided custom slug - validate and check uniqueness
		if err := s.validateSlug(*slug); err != nil {
			s.storage.Delete(storagePath) // Clean up on error
			return nil, err
		}
		if err := s.checkSlugUnique(*slug); err != nil {
			s.storage.Delete(storagePath) // Clean up on error
			return nil, err
		}
		fileSlug = *slug
		// Make original filename unique if duplicate exists
		uniqueOriginalName = s.makeOriginalNameUnique(fileHeader.Filename, uniqueFilename)
	} else {
		// No custom slug provided - use original filename as slug
		// Make both slug and original name unique together (same value)
		uniqueOriginalName, err = s.makeFilenameAndSlugUnique(fileHeader.Filename, uniqueFilename)
		if err != nil {
			s.storage.Delete(storagePath) // Clean up on error
			return nil, fmt.Errorf("failed to generate unique filename: %w", err)
		}
		fileSlug = uniqueOriginalName // Slug is the same as the unique original name
	}

	// Create database record
	file := &models.File{
		Filename:     uniqueFilename,
		OriginalName: uniqueOriginalName,
		FilePath:     storagePath,
		FileSize:     fileHeader.Size,
		ContentType:  fileHeader.Header.Get("Content-Type"),
		Slug:         fileSlug,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
	}

	if err := database.DB.Create(file).Error; err != nil {
		s.storage.Delete(storagePath) // Clean up on error
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

// GetFileByOriginalName retrieves a file by its original filename
func (s *FileService) GetFileByOriginalName(originalName string) (*models.File, error) {
	var file models.File
	if err := database.DB.Where("original_name = ?", originalName).First(&file).Error; err != nil {
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

// DeleteFile deletes a file from storage and database
func (s *FileService) DeleteFile(id uint) error {
	file, err := s.GetFile(id)
	if err != nil {
		return err
	}

	// Delete file from storage
	if err := s.storage.Delete(file.FilePath); err != nil {
		return fmt.Errorf("failed to delete file from storage: %w", err)
	}

	// Delete from database (soft delete)
	if err := database.DB.Delete(file).Error; err != nil {
		return fmt.Errorf("failed to delete from database: %w", err)
	}

	return nil
}

// GetFileReader returns a reader for the file content from storage
func (s *FileService) GetFileReader(file *models.File) (io.ReadCloser, error) {
	return s.storage.Get(file.FilePath)
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

// CleanupExpiredFiles removes expired files from storage and database
func (s *FileService) CleanupExpiredFiles() error {
	var expiredFiles []models.File
	if err := database.DB.Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Find(&expiredFiles).Error; err != nil {
		return err
	}

	for _, file := range expiredFiles {
		// Delete file from storage
		if err := s.storage.Delete(file.FilePath); err != nil {
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

// makeOriginalNameUnique ensures the original filename is unique by appending hex prefix if needed
func (s *FileService) makeOriginalNameUnique(originalName, uniqueFilename string) string {
	// Check if original name already exists
	var count int64
	database.DB.Model(&models.File{}).Where("original_name = ?", originalName).Count(&count)

	if count == 0 {
		// No duplicate, return as-is
		return originalName
	}

	// Duplicate found - append first 5 chars of unique filename
	ext := filepath.Ext(originalName)
	basename := strings.TrimSuffix(originalName, ext)

	// Extract first 5 chars from the hex filename (excluding extension)
	hexFilename := strings.TrimSuffix(uniqueFilename, filepath.Ext(uniqueFilename))
	prefix := ""
	if len(hexFilename) >= 5 {
		prefix = hexFilename[:5]
	} else {
		prefix = hexFilename
	}

	return fmt.Sprintf("%s-%s%s", basename, prefix, ext)
}

// makeFilenameAndSlugUnique ensures both the original filename and slug are unique (returns same value for both)
func (s *FileService) makeFilenameAndSlugUnique(originalName, uniqueFilename string) (string, error) {
	// Check if original name already exists in either original_name or slug columns
	var count int64
	database.DB.Model(&models.File{}).Where("original_name = ? OR slug = ?", originalName, originalName).Count(&count)

	if count == 0 {
		// No duplicate, return as-is
		return originalName, nil
	}

	// Duplicate found - append random suffix before extension
	ext := filepath.Ext(originalName)
	basename := strings.TrimSuffix(originalName, ext)

	// Try up to 100 times to find a unique name
	for i := 0; i < 100; i++ {
		// Generate random suffix
		randomBytes := make([]byte, 2)
		if _, err := rand.Read(randomBytes); err != nil {
			return "", err
		}
		suffix := hex.EncodeToString(randomBytes)

		uniqueName := fmt.Sprintf("%s-%s%s", basename, suffix, ext)

		// Check if this is unique
		database.DB.Model(&models.File{}).Where("original_name = ? OR slug = ?", uniqueName, uniqueName).Count(&count)
		if count == 0 {
			return uniqueName, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique filename after 100 attempts")
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
	// Keep the full filename including extension as the slug
	slug := filename

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Remove consecutive hyphens
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")

	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")

	// If slug is empty, generate random
	if slug == "" {
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

		// Append random suffix (before extension if it exists)
		ext := filepath.Ext(originalSlug)
		basename := strings.TrimSuffix(originalSlug, ext)
		randomBytes := make([]byte, 2)
		rand.Read(randomBytes)
		slug = basename + "-" + hex.EncodeToString(randomBytes) + ext
	}

	return "", fmt.Errorf("failed to generate unique slug")
}
