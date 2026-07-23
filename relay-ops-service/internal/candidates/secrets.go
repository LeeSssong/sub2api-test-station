package candidates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrSecretConflict = errors.New("candidate secret already exists")

type SecretStore interface {
	Install(string, []byte) (string, error)
	Remove(string) error
}

type FileSecretStore struct{ Directory string }

func (s FileSecretStore) Install(name string, rawKey []byte) (string, error) {
	directory, err := s.validDirectory()
	if err != nil {
		return "", err
	}
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return "", fmt.Errorf("candidate name is required")
	}
	key := bytes.TrimSpace(rawKey)
	if len(key) < 4 || len(key) > MaxProbeKeyBytes {
		return "", fmt.Errorf("candidate probe key size is invalid")
	}
	digest := sha256.Sum256([]byte(normalizedName))
	path := filepath.Join(directory, hex.EncodeToString(digest[:])+".key")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return "", ErrSecretConflict
	}
	if err != nil {
		return "", fmt.Errorf("create candidate secret")
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	written, err := file.Write(key)
	if err != nil || written != len(key) {
		return "", fmt.Errorf("write candidate secret")
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync candidate secret")
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close candidate secret")
	}
	cleanup = false
	return path, nil
}

func (s FileSecretStore) Remove(path string) error {
	directory, err := s.validDirectory()
	if err != nil {
		return err
	}
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(cleanPath) || filepath.Dir(cleanPath) != directory {
		return fmt.Errorf("candidate secret path is outside managed directory")
	}
	if err := os.Remove(cleanPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove candidate secret")
	}
	return nil
}

func (s FileSecretStore) validDirectory() (string, error) {
	raw := strings.TrimSpace(s.Directory)
	if raw == "" {
		return "", fmt.Errorf("candidate secret directory is required")
	}
	directory, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", fmt.Errorf("resolve candidate secret directory")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", fmt.Errorf("candidate secret directory is unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("candidate secret directory must be a non-symlink directory with mode 0700")
	}
	return directory, nil
}
