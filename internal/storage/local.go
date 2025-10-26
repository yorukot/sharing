package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage implements the Storage interface using the local filesystem
type LocalStorage struct {
	dataDir string
}

// NewLocalStorage creates a new local filesystem storage backend
func NewLocalStorage(dataDir string) (*LocalStorage, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return &LocalStorage{
		dataDir: dataDir,
	}, nil
}

// Save saves a file to the local filesystem
func (l *LocalStorage) Save(reader io.Reader, filename string, size int64) (string, error) {
	filePath := filepath.Join(l.dataDir, filename)

	// Create destination file
	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, reader); err != nil {
		os.Remove(filePath) // Clean up on error
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	return filePath, nil
}

// Get retrieves a file from the local filesystem
func (l *LocalStorage) Get(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return file, nil
}

// Delete removes a file from the local filesystem
func (l *LocalStorage) Delete(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// Exists checks if a file exists on the local filesystem
func (l *LocalStorage) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}
	return true, nil
}
