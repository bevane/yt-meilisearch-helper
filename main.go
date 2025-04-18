package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"github.com/meilisearch/meilisearch-go"
)

func main() {
	isUpdate := flag.Bool("u", false, "update details of videos in queue and set them to be reindexed")
	flag.Parse()

	godotenv.Load(".env")
	dataPath := os.Getenv("DATA_PATH")
	channelUrl := os.Getenv("CHANNEL_URL")
	whisperModelPath := os.Getenv("WHISPER_MODEL_PATH")

	searchClient, err := meilisearch.Connect(os.Getenv("MEILISEARCH_URL"), meilisearch.WithAPIKey(os.Getenv("MEILISEARCH_API_KEY")))
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to connect to meilisearch: %s\n", err.Error()))
	}

	slog.Info(fmt.Sprintf("Setting project directory to %s", dataPath))
	slog.Info(fmt.Sprintf("Downloading and Processing videos for %s", channelUrl))
	err = initDataDir(dataPath)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to initialize project folder: %v", err.Error()))
		os.Exit(1)
	}

	videosJsonData, err := os.ReadFile(filepath.Join(dataPath, "videos.json"))
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to read videos.json: %v", err.Error()))
		os.Exit(1)
	}

	safeVideoDataCollection := SafeVideoDataCollection{}
	initialVideoDataCollection := VideoDataCollection{}
	err = json.Unmarshal(videosJsonData, &initialVideoDataCollection)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to unmarshall videos.json: %v", err.Error()))
		os.Exit(1)
	}

	safeVideoDataCollection.videosDataAndStatus = initialVideoDataCollection

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		saveProgress(dataPath, &safeVideoDataCollection)
		printSummary(&safeVideoDataCollection)

		os.Exit(130)
	}()

	err = gatherVideos(channelUrl, *isUpdate, &safeVideoDataCollection)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to gather videos: %v", err.Error()))
	}

	printSummary(&safeVideoDataCollection)

	downloadDir := filepath.Join(dataPath, "downloads")
	processedDir := filepath.Join(dataPath, "processed")
	transcriptsDir := filepath.Join(dataPath, "transcripts")

	downloadQueue := make(chan string)
	processQueue := make(chan string)
	transcribeQueue := make(chan string)
	indexQueue := make(chan string)

	var wg sync.WaitGroup

	for range 2 {
		go downloadWorker(downloadQueue, processQueue, downloadDir, &safeVideoDataCollection)
		go processWorker(processQueue, transcribeQueue, downloadDir, processedDir, &safeVideoDataCollection)
		go transcribeWorker(transcribeQueue, indexQueue, processedDir, transcriptsDir, whisperModelPath, &safeVideoDataCollection, &wg)
	}

	go indexWorker(indexQueue, transcriptsDir, searchClient, &safeVideoDataCollection, &wg)

	for id, video := range safeVideoDataCollection.videosDataAndStatus {
		switch video.Status {
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
		case "transcribed":
			slog.Info("Adding to index queue")
			wg.Add(1)
			indexQueue <- id
		case "indexed":
		default:
			slog.Error(fmt.Sprintf("Unexpected video status: %s", video.Status))
		}

		if video.ReIndex && video.Status == "indexed" {
			slog.Info("Adding to index queue (reindex)")
			wg.Add(1)
			indexQueue <- id
		}
	}

	wg.Wait()

	saveProgress(dataPath, &safeVideoDataCollection)
	printSummary(&safeVideoDataCollection)
}
