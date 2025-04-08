package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cleanup(dataPath)
		saveProgress(dataPath, videoProcessingStatus)

		os.Exit(130)
	}()

	err = gatherVideos(channelUrl, videoProcessingStatus)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to gather videos: %v", err.Error()))
	}

	for k, v := range videoProcessingStatus {
		switch v {
		case "pending":
			downloadDir := filepath.Join(dataPath, "downloads")
			downloadVideo(k, videoProcessingStatus, downloadDir)
		default:
			slog.Error(fmt.Sprintf("Unexpected video status: %s", v))

		}
	}

	cleanup(dataPath)
	saveProgress(dataPath, videoProcessingStatus)

}
