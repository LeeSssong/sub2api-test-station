package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrCorruptState = errors.New("updater state is corrupt")

// Store persists the one active update operation using atomic file replacement.
type Store struct {
	path string
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() (*Operation, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read updater state: %w", err)
	}
	var op Operation
	if err := json.Unmarshal(data, &op); err != nil || op.OperationID == "" || op.SchemaVersion != stateSchemaVersion {
		return nil, fmt.Errorf("%w: %v", ErrCorruptState, err)
	}
	return &op, nil
}

func (s *Store) Save(op Operation) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create updater state directory: %w", err)
	}
	data, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("encode updater state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return fmt.Errorf("create updater state temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod updater state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write updater state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync updater state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close updater state: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace updater state: %w", err)
	}
	return os.Chmod(s.path, 0o600)
}

func (s *Store) Clear() error {
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove updater state: %w", err)
	}
	return nil
}
