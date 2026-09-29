package server

import (
	"os"
	"path/filepath"
)

const (
	BundledPublicDir = "./public"
	RuntimePublicDir = "/app/runtime/public"
)

func ActivePublicDir() string {
	return SelectPublicDir(RuntimePublicDir, BundledPublicDir)
}

func SelectPublicDir(updatedDir, bundledDir string) string {
	info, err := os.Stat(filepath.Join(updatedDir, "index.html"))
	if err == nil && !info.IsDir() {
		return updatedDir
	}
	return bundledDir
}

func PublicAssetPath(name string) string {
	return filepath.Join(ActivePublicDir(), name)
}
