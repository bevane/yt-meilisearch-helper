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

// when adding new fields, the gatherVideos and IndexWorker functions
// have to be updated to assign values to the new fields
type VideoDetails struct {
	Id    string `json:"id"`
	Title string `json:"title"`
}

type VideosDataAndStatus map[string]struct {
	Status  string `json:"status"`
	ReIndex bool   `json:"reIndex"`
	VideoDetails
}

type Document struct {
	Transcript string `json:"transcript"`
	VideoDetails
}

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

	videosDataAndStatus := VideosDataAndStatus{}
	err = json.Unmarshal(videosJsonData, &videosDataAndStatus)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to unmarshall videos.json: %v", err.Error()))
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cleanup(dataPath)
		saveProgress(dataPath, videosDataAndStatus)

		os.Exit(130)
	}()

	err = gatherVideos(channelUrl, *isUpdate, videosDataAndStatus)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to gather videos: %v", err.Error()))
	}

	downloadDir := filepath.Join(dataPath, "downloads")
	processedDir := filepath.Join(dataPath, "processed")
	transcriptsDir := filepath.Join(dataPath, "transcripts")

	downloadQueue := make(chan string)
	processQueue := make(chan string)
	transcribeQueue := make(chan string)
	indexQueue := make(chan string)

	var wg sync.WaitGroup

	for range 5 {
		go downloadWorker(downloadQueue, processQueue, downloadDir, videosDataAndStatus)
		go processWorker(processQueue, transcribeQueue, downloadDir, processedDir, videosDataAndStatus)
	}

	for range 2 {
		go transcribeWorker(transcribeQueue, indexQueue, processedDir, transcriptsDir, whisperModelPath, videosDataAndStatus, &wg)
	}

	go indexWorker(indexQueue, transcriptsDir, searchClient, videosDataAndStatus, &wg)

	for id, video := range videosDataAndStatus {
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

	cleanup(dataPath)
	saveProgress(dataPath, videosDataAndStatus)
}
