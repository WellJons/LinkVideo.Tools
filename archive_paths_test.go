package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChooseArchiveFolderPrefersIndex(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	real := filepath.Join(root, "real")
	os.MkdirAll(empty, 0755)
	os.MkdirAll(filepath.Join(real, "linkvideo_301547", "1794310053262"), 0755)
	os.WriteFile(filepath.Join(real, "linkvideo_301547", "1794310053262", "nimble-dvr-segments.db"), []byte("{}"), 0600)
	if got := chooseArchiveFolder([]string{empty, real}); got != real {
		t.Fatalf("got %q, expected %q", got, real)
	}
}
func TestChooseArchiveFolderFallback(t *testing.T) {
	root := t.TempDir()
	if got := chooseArchiveFolder([]string{root}); got != root {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeVideoRoot(t *testing.T) {
	base := t.TempDir()
	videos := filepath.Join(base, "Videos")
	inside := filepath.Join(videos, "linkvideo_9999", "1234")
	if err := os.MkdirAll(inside, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "nimble-dvr-segments.db"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := normalizedVideoRoot(base); got != videos {
		t.Errorf("parent selection=%q", got)
	}
	if got := normalizedVideoRoot(filepath.Join(videos, "linkvideo_9999")); got != videos {
		t.Errorf("camera selection=%q", got)
	}
}
