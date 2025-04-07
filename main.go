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
	slog.Info(fmt.Sprintf("Setting project directory to %s", dataPath))
	slog.Info(fmt.Sprintf("Downloading and Processing videos for %s", channelUrl))
	err := initDataDir(dataPath)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to initialize project folder: %v", err.Error()))
		os.Exit(1)
	}

	progressData, err := os.ReadFile(filepath.Join(dataPath, "progress.json"))
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to read progress.json: %v", err.Error()))
		os.Exit(1)
	}

	videoProcessingStatus := VideoProcessingStatus{}
	err = json.Unmarshal(progressData, &videoProcessingStatus)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to unmarshall progress.json: %v", err.Error()))
		os.Exit(1)
	}

	err = gatherVideos(channelUrl, videoProcessingStatus)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to gather videos: %v", err.Error()))
	}

	updatedProgressData, err := json.MarshalIndent(videoProcessingStatus, "", "\t")
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to marshall progress.json data: %v", err.Error()))
		os.Exit(1)
	}

	err = os.WriteFile(filepath.Join(dataPath, "progress.json"), updatedProgressData, 0666)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to write to progress.json: %v", err.Error()))
		os.Exit(1)
	}
}
