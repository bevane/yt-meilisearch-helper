package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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

	if err != nil {
		slog.Error(fmt.Sprintf("Error : %v\n", err.Error()))
		os.Exit(1)
	}

	progressData, err := os.ReadFile(filepath.Join(dataPath, "progress.json"))

	if err != nil {
		slog.Error(fmt.Sprintf("Error reading progress.json: %v\n", err.Error()))
		os.Exit(1)
	}

	videoProcessingStatus := VideoProcessingStatus{}
	err = json.Unmarshal(progressData, &videoProcessingStatus)
	if err != nil {
		slog.Error(fmt.Sprintf("Error unmarshalling progress.json: %v\n", err.Error()))
		os.Exit(1)
	}

	err = gatherVideos(channelUrl, videoProcessingStatus)
	if err != nil {
		slog.Error(fmt.Sprintf("Error gathering videos: %v\n", err.Error()))
		os.Exit(1)
	}

	updatedProgressData, err := json.MarshalIndent(videoProcessingStatus, "", "\t")
	if err != nil {
		slog.Error(fmt.Sprintf("Error marshalling progress.json data: %v\n", err.Error()))
		os.Exit(1)
	}

	err = os.WriteFile(filepath.Join(dataPath, "progress.json"), updatedProgressData, 0666)
	if err != nil {
		slog.Error(fmt.Sprintf("Error writing progress.json: %v\n", err.Error()))
		os.Exit(1)
	}
}
