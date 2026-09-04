package sonda

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// WorkspaceDir is the stable path of the second working copy kept for a
// repository, where the baseline is built. It stays put between reviews: a
// build finds its own output and the compiler's cache where it left them,
// keyed in part by where the sources sit, so a directory made fresh each time
// pays for a cold build every time.
//
// It sits under the cache directory, named after the repository and told
// apart from another checkout of the same name by a hash of the root.
func WorkspaceDir(root string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(root))
	name := filepath.Base(root) + "-" + hex.EncodeToString(sum[:4])
	return filepath.Join(base, "peneira", "sonda", name), nil
}
