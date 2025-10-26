package models

import (
	"time"

	"gorm.io/gorm"
)

// File represents a shared file in the system
type File struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// File metadata
	Filename     string `gorm:"uniqueIndex;not null" json:"filename"` // Unique stored filename
	OriginalName string `gorm:"not null" json:"original_name"`        // Original uploaded filename
	FilePath     string `gorm:"not null" json:"-"`                    // Full path on disk
	FileSize     int64  `gorm:"not null" json:"file_size"`            // Size in bytes
	ContentType  string `gorm:"not null" json:"content_type"`         // MIME type

	// Short link / slug for public sharing
	Slug string `gorm:"uniqueIndex;not null" json:"slug"` // URL-safe short link (e.g., "demo-file")

	// Security and access control
	PasswordHash *string    `json:"-"`                                 // Bcrypt hash (nullable)
	ExpiresAt    *time.Time `gorm:"index" json:"expires_at,omitempty"` // Expiration time (nullable)
}

// IsExpired checks if the file has expired
func (f *File) IsExpired() bool {
	if f.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*f.ExpiresAt)
}

// HasPassword checks if the file is password protected
func (f *File) HasPassword() bool {
	return f.PasswordHash != nil && *f.PasswordHash != ""
}
