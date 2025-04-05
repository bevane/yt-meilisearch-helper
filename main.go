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
	channelUrl := os.Getenv("CHANNEL_URL")
	fmt.Printf("Setting project directory to %s\n", dataPath)
	fmt.Printf("Downloading and Processing videos for %s\n", channelUrl)
	err := initDataDir(dataPath)
	if err != nil {
		slog.Error(fmt.Sprintf("Error initializing project folder: %v\n", err.Error()))
		os.Exit(1)
	}
}
