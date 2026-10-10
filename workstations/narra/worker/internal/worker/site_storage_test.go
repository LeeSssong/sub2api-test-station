package worker

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSiteLocalStorageProtectedURL(t *testing.T) {
	t.Setenv("LOCAL_MEDIA_ROOT", t.TempDir())
	s, err := NewStorage(context.Background(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	image, err := s.PersistImage(context.Background(), "site_1_12345678-1234-1234-1234-123456789abc", []byte("image"), "png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(image.URL, "/image-workstation/api/workstation/media/") {
		t.Fatalf("unprotected URL %s", image.URL)
	}
	if image.MediaStorage != MediaStorageLocal {
		t.Fatal("local media must report LOCAL storage")
	}
	if _, err := os.Stat(os.Getenv("LOCAL_MEDIA_ROOT") + "/" + image.StorageKey); err != nil {
		t.Fatal(err)
	}
}
