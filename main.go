package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"github.com/meilisearch/meilisearch-go"
)

type VideoProcessingStatus map[string]string

func main() {

	godotenv.Load(".env")
	dataPath := os.Getenv("DATA_PATH")
	channelUrl := os.Getenv("CHANNEL_URL")
	whisperModelPath := os.Getenv("WHISPER_MODEL_PATH")

	_ = meilisearch.New(os.Getenv("MEILISEARCH_URL"), meilisearch.WithAPIKey(os.Getenv("MEILISEARCH_API_KEY")))

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

	videoProgress := VideoProcessingStatus{}
	err = json.Unmarshal(progressData, &videoProgress)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to unmarshall progress.json: %v", err.Error()))
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cleanup(dataPath)
		saveProgress(dataPath, videoProgress)

		os.Exit(130)
	}()

	err = gatherVideos(channelUrl, videoProgress)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to gather videos: %v", err.Error()))
	}

	downloadDir := filepath.Join(dataPath, "downloads")
	processedDir := filepath.Join(dataPath, "processed")
	transcriptsDir := filepath.Join(dataPath, "transcripts")

	downloadQueue := make(chan string)
	processQueue := make(chan string)
	transcribeQueue := make(chan string)

	var wg sync.WaitGroup

	for range 5 {
		go downloadWorker(downloadQueue, processQueue, downloadDir, videoProgress)
		go processWorker(processQueue, transcribeQueue, downloadDir, processedDir, videoProgress)
	}

	for range 2 {
		go transcribeWorker(transcribeQueue, processedDir, transcriptsDir, whisperModelPath, videoProgress, &wg)
	}

	for id, status := range videoProgress {
		switch status {
		case "pending":
			slog.Info("Adding to download queue")
			wg.Add(1)
			downloadQueue <- id
		case "downloaded":
			slog.Info("Adding to process queue")
			wg.Add(1)
			processQueue <- id
		case "processed":
			slog.Info("Adding to transcribe queue")
			wg.Add(1)
			transcribeQueue <- id
		default:
			slog.Error(fmt.Sprintf("Unexpected video status: %s", status))
		}
	}

	wg.Wait()

	cleanup(dataPath)
	saveProgress(dataPath, videoProgress)
}
