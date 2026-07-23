package candidates

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSecretStoreInstalls0600FileWithoutNameTraversal(t *testing.T) {
	t.Parallel()

	directory := secureDirectory(t)
	store := FileSecretStore{Directory: directory}
	key := []byte("  sk-candidate-secret-1234\n")

	path, err := store.Install("../../Candidate A", key)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if filepath.Dir(path) != directory {
		t.Fatalf("path escaped managed directory: %q", path)
	}
	if strings.Contains(filepath.Base(path), "Candidate") || strings.Contains(filepath.Base(path), "..") {
		t.Fatalf("filename contains untrusted name: %q", filepath.Base(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "sk-candidate-secret-1234" {
		t.Fatalf("stored key = %q", raw)
	}
}

func TestFileSecretStoreRejectsInvalidDirectoryKeyAndOverwrite(t *testing.T) {
	t.Parallel()

	t.Run("directory permissions", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Chmod(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := (FileSecretStore{Directory: directory}).Install("candidate", []byte("valid-key"))
		if err == nil {
			t.Fatal("Install accepted directory mode 0755")
		}
	})

	t.Run("directory symlink", func(t *testing.T) {
		target := secureDirectory(t)
		link := filepath.Join(t.TempDir(), "managed-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		_, err := (FileSecretStore{Directory: link}).Install("candidate", []byte("valid-key"))
		if err == nil {
			t.Fatal("Install accepted a symlink directory")
		}
	})

	t.Run("short key", func(t *testing.T) {
		_, err := (FileSecretStore{Directory: secureDirectory(t)}).Install("candidate", []byte("abc"))
		if err == nil {
			t.Fatal("Install accepted a three-byte key")
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		store := FileSecretStore{Directory: secureDirectory(t)}
		if _, err := store.Install("Candidate", []byte("first-key")); err != nil {
			t.Fatal(err)
		}
		_, err := store.Install(" candidate ", []byte("second-key"))
		if !errors.Is(err, ErrSecretConflict) {
			t.Fatalf("error = %v, want ErrSecretConflict", err)
		}
	})
}

func TestFileSecretStoreRemoveStaysInsideManagedDirectory(t *testing.T) {
	t.Parallel()

	directory := secureDirectory(t)
	store := FileSecretStore{Directory: directory}
	managed, err := store.Install("candidate", []byte("managed-key"))
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-key")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(outside); err == nil {
		t.Fatal("Remove accepted an outside path")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file changed: %v", err)
	}
	if err := store.Remove(managed); err != nil {
		t.Fatalf("Remove managed: %v", err)
	}
	if _, err := os.Stat(managed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed file remains: %v", err)
	}
	if err := store.Remove(managed); err != nil {
		t.Fatalf("Remove missing managed file: %v", err)
	}
}

func secureDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}
