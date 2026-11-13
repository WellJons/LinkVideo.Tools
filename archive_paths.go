package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Return the actual DVR folder, not merely the first existing Documents directory.
func archiveRootHasIndex(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(strings.ToLower(e.Name()), "linkvideo_") {
			continue
		}
		subs, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		for _, sub := range subs {
			if !sub.IsDir() {
				continue
			}
			path := filepath.Join(root, e.Name(), sub.Name(), "nimble-dvr-segments.db")
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				return true
			}
		}
	}
	return false
}
func chooseArchiveFolder(candidates []string) string {
	seen := map[string]bool{}
	existing := ""
	for _, c := range candidates {
		if c == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(c))
		if seen[key] {
			continue
		}
		seen[key] = true
		if archiveRootHasIndex(c) {
			return c
		}
		if existing == "" {
			if st, err := os.Stat(c); err == nil && st.IsDir() {
				existing = c
			}
		}
	}
	return existing
}
func archiveCandidates() []string {
	result := []string{}
	add := func(root string) {
		if root != "" {
			result = append(result, filepath.Join(root, "LinkVideo.Server", "Videos"))
		}
	}
	add(windowsDocumentsFolder())
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "Documents"))
		add(filepath.Join(home, "Документы"))
	}
	if root := os.Getenv("OneDrive"); root != "" {
		add(filepath.Join(root, "Documents"))
		add(filepath.Join(root, "Документы"))
	}
	if root := os.Getenv("OneDriveCommercial"); root != "" {
		add(filepath.Join(root, "Documents"))
	}
	if root := os.Getenv("PUBLIC"); root != "" {
		add(filepath.Join(root, "Documents"))
	}
	return result
}

// Manual folder selection may point to LinkVideo.Server rather than its Videos subfolder.
func normalizedVideoRoot(selected string) string {
	if archiveRootHasIndex(selected) {
		return selected
	}
	child := filepath.Join(selected, "Videos")
	if archiveRootHasIndex(child) {
		return child
	}
	if strings.HasPrefix(strings.ToLower(filepath.Base(selected)), "linkvideo_") {
		parent := filepath.Dir(selected)
		if archiveRootHasIndex(parent) {
			return parent
		}
	}
	return selected
}
