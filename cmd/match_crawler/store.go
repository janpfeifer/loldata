package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/janpfeifer/loldata/data"
)

// DatasetStore manages persistent saving and loading of the dataset,
// ensuring atomic writes via temporary files, file locking, and rolling backups.
type DatasetStore struct {
	mu       sync.Mutex
	filePath string
	tempPath string
	lockPath string
	backups  int
}

// NewDatasetStore initializes a new DatasetStore for the given file path and backup count.
func NewDatasetStore(filePath string, backups int) *DatasetStore {
	return &DatasetStore{
		filePath: filePath,
		tempPath: filePath + "~",
		lockPath: filePath + ".lock",
		backups:  backups,
	}
}

// FilePath returns the configured dataset file path.
func (s *DatasetStore) FilePath() string {
	return s.filePath
}

// Exists checks if the dataset file currently exists on disk.
func (s *DatasetStore) Exists() bool {
	info, err := os.Stat(s.filePath)
	return err == nil && !info.IsDir()
}

// Load loads the dataset from filePath into ds if the file exists.
// If the file does not exist, it does nothing and returns nil.
func (s *DatasetStore) Load(ds *data.Dataset) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.filePath == "" {
		return nil
	}

	if !s.Exists() {
		return nil
	}

	// Acquire shared file lock for reading
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file %q: %w", s.lockPath, err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_SH); err != nil {
		return fmt.Errorf("failed to acquire shared lock on %q: %w", s.lockPath, err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	return ds.LoadFromJSON(s.filePath)
}

// Save atomically writes the dataset to disk with file locking and manages backup rotation.
func (s *DatasetStore) Save(ds *data.Dataset) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.filePath == "" {
		return nil
	}

	// Ensure parent directory exists
	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
	}

	// Acquire exclusive file lock
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file %q: %w", s.lockPath, err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire exclusive lock on %q: %w", s.lockPath, err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	// Step 1: Write dataset to temporary file <filePath>~
	tempFile, err := os.Create(s.tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file %q: %w", s.tempPath, err)
	}

	writer := bufio.NewWriter(tempFile)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(ds); err != nil {
		tempFile.Close()
		_ = os.Remove(s.tempPath)
		return fmt.Errorf("failed to encode dataset to JSON: %w", err)
	}

	if err := writer.Flush(); err != nil {
		tempFile.Close()
		_ = os.Remove(s.tempPath)
		return fmt.Errorf("failed to flush data to temporary file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		_ = os.Remove(s.tempPath)
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(s.tempPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Step 2: Handle backups if previous dataset file exists and backups > 0
	if s.backups > 0 && s.Exists() {
		ts := time.Now().Format("20060102150405")
		backupPath := fmt.Sprintf("%s.backup-%s", s.filePath, ts)

		// Handle collision if multiple saves in the exact same second
		if _, err := os.Stat(backupPath); err == nil {
			for suffix := 1; ; suffix++ {
				candidate := fmt.Sprintf("%s_%d", backupPath, suffix)
				if _, err := os.Stat(candidate); os.IsNotExist(err) {
					backupPath = candidate
					break
				}
			}
		}

		// Move existing dataset file to backup path
		if err := os.Rename(s.filePath, backupPath); err != nil {
			_ = os.Remove(s.tempPath)
			return fmt.Errorf("failed to create backup %q: %w", backupPath, err)
		}

		// Prune old backups to keep only the latest s.backups
		s.pruneBackups(dir)
	}

	// Step 3: Atomically move temporary file <filePath>~ to <filePath>
	if err := os.Rename(s.tempPath, s.filePath); err != nil {
		return fmt.Errorf("failed to atomically rename %q to %q: %w", s.tempPath, s.filePath, err)
	}

	return nil
}

// pruneBackups keeps only the newest `s.backups` files matching `<baseName>.backup-*`.
func (s *DatasetStore) pruneBackups(dir string) {
	if s.backups <= 0 {
		return
	}

	baseName := filepath.Base(s.filePath)
	prefix := baseName + ".backup-"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var backupFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			backupFiles = append(backupFiles, filepath.Join(dir, entry.Name()))
		}
	}

	if len(backupFiles) <= s.backups {
		return
	}

	// Sort alphabetically which corresponds to chronological order (backup-YYYYMMDDhhmmss)
	sort.Strings(backupFiles)

	numToDelete := len(backupFiles) - s.backups
	for i := 0; i < numToDelete; i++ {
		_ = os.Remove(backupFiles[i])
	}
}
