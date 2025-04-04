package main

import (
	"log/slog"
	"os"
	"path/filepath"
)

func initDataDir(dataPath string) {
	progressPath := filepath.Join(dataPath, "progress.json")
	if _, err := os.Stat(progressPath); os.IsNotExist(err) {
		slog.Info("progress.json not found, creating progress.json")
		os.Create(progressPath)
	}
}
