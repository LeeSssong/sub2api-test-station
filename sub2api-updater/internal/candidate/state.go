package candidate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func writeState(path string, result Result) error {
	if path == "" {
		return errors.New("candidate state path is required")
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errors.New("encode candidate state")
	}
	data = append(data, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return errors.New("create candidate state directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return errors.New("secure candidate state directory")
	}
	file, err := os.CreateTemp(directory, ".candidate-state-*.tmp")
	if err != nil {
		return errors.New("create candidate state")
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return errors.New("secure candidate state")
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return errors.New("write candidate state")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return errors.New("sync candidate state")
	}
	if err := file.Close(); err != nil {
		return errors.New("close candidate state")
	}
	if err := os.Rename(tempPath, path); err != nil {
		return errors.New("promote candidate state")
	}
	return os.Chmod(path, 0o600)
}
