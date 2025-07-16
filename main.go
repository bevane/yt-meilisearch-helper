package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/joho/godotenv"
	"github.com/meilisearch/meilisearch-go"
)

func main() {
	isUpdate := flag.Bool("u", false, "update details/metadata of videos in queue and set them to be reindexed")
	flag.Parse()

	godotenv.Load(".env")
	dataPath := os.Getenv("DATA_PATH")
	if dataPath == "" {
		slog.Error(fmt.Sprintln("DATA_PATH env variable is not set"))
		os.Exit(1)
	}
	channelUrl := os.Getenv("CHANNEL_URL")
	if channelUrl == "" {
		slog.Error(fmt.Sprintln("CHANNEL_URL env variable is not set"))
		os.Exit(1)
	}
	whisperModelPath := os.Getenv("WHISPER_MODEL_PATH")
	if whisperModelPath == "" {
		slog.Error(fmt.Sprintln("WHISPER_MODEL_PATH env variable is not set"))
		os.Exit(1)
	}
	maxDownloadAndProcessWorkers, err := strconv.Atoi(os.Getenv("MAX_DOWNLOAD_PROCESS_WORKERS"))
	if err != nil {
		slog.Error(fmt.Sprintf("MAX_DOWNLOAD_PROCESS_WORKERS env variable is invalid: %s", err.Error()))
		os.Exit(1)
	}
	maxVideoDetailFetchWorkers, err := strconv.Atoi(os.Getenv("MAX_VIDEO_DETAIL_FETCH_WORKERS"))
	if err != nil {
		slog.Error(fmt.Sprintf("MAX_VIDEO_DETAIL_FETCH_WORKERS env variable is invalid: %s", err.Error()))
		os.Exit(1)
	}

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

	// progress for each video is saved in videos.json
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

	// gracefully shutdown on interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		saveProgress(dataPath, &safeVideoDataCollection)
		printSummary(&safeVideoDataCollection)

		os.Exit(130)
	}()

	err = gatherVideos(channelUrl, *isUpdate, &safeVideoDataCollection, maxVideoDetailFetchWorkers)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to gather videos: %v", err.Error()))
	}

	printSummary(&safeVideoDataCollection)

	downloadDir := filepath.Join(dataPath, "downloads")
	// the downloaded file has to be converted to a specific format for
	// whisper to transcribe it (check whisper.cpp documentation for details)
	processedDir := filepath.Join(dataPath, "processed")
	transcriptsDir := filepath.Join(dataPath, "transcripts")

	downloadQueue := make(chan string)
	processQueue := make(chan string)
	transcribeQueue := make(chan string)
	indexQueue := make(chan string)

	var wg sync.WaitGroup

	// n+1 (n = number of transcribe workers) concurrent workers for downloading and processing is sufficient
	// as transcribing is the bottleneck. This can be increased to create
	// a larger buffer of downloaded and processed videos
	for range maxDownloadAndProcessWorkers {
		go downloadWorker(downloadQueue, processQueue, downloadDir, &safeVideoDataCollection)
		go processWorker(processQueue, transcribeQueue, downloadDir, processedDir, &safeVideoDataCollection)
	}

	// 1 is recommended, can be increased if more system resources are available to run multiple LLM processes at the same time
	for range 2 {
		go transcribeWorker(transcribeQueue, indexQueue, processedDir, transcriptsDir, whisperModelPath, &safeVideoDataCollection, &wg)
	}

	// indexWorker uploades batches of json files to meilisearch, hence
	// one worker is sufficient
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
