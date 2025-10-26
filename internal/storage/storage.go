package storage

import (
	"io"
)

// Storage defines the interface for file storage backends
type Storage interface {
	// Save saves a file with the given filename and returns the storage path/key
	Save(reader io.Reader, filename string, size int64) (string, error)

	// Get retrieves a file by its storage path/key and returns a reader
	Get(path string) (io.ReadCloser, error)

	// Delete removes a file from storage
	Delete(path string) error

	// Exists checks if a file exists in storage
	Exists(path string) (bool, error)
}
