// Package util — мелкие общие помощники.
package util

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome раскрывает ведущую ~ в пути.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}

	return p
}
