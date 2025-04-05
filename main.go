package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

type VideoProcessingStatus map[string]string

func main() {
	godotenv.Load(".env")
	dataPath := os.Getenv("DATA_PATH")
	err := initDataDir(dataPath)
	if err != nil {
		slog.Error(fmt.Sprintf("Error initializing project folder: %v", err.Error()))
		os.Exit(1)
	}
}
