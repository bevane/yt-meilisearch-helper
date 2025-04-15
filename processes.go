package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

func initDataDir(dataPath string) error {
	videoDataPath := filepath.Join(dataPath, "videos.json")
	downloadsPath := filepath.Join(dataPath, "downloads")
	processedPath := filepath.Join(dataPath, "processed")
	transcriptsPath := filepath.Join(dataPath, "transcripts")

	// the logic for creating files and directories will be different
	// because os.Mkdir and os.Create return errors differently in case
	// the file or directory already exists
	// os.Create does not throw error if the file already exists and instead
	// will truncate the file so check if file exists explicitly with os.Stat
	_, err := os.Stat(videoDataPath)
	if err != nil && os.IsNotExist(err) {
		slog.Info("videos.json not found, creating videos.json")
		progressFile, err := os.Create(videoDataPath)
		if err != nil {
			return err
		}
		defer progressFile.Close()
		_, err = progressFile.Write([]byte("{}"))
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// os.Mkdir returns an error if the directory already exists
	// so only create dir if the error is nil
	err = os.Mkdir(downloadsPath, 0750)
	if err != nil && !os.IsExist(err) {
		return err
	} else if err == nil {
		slog.Info("downloads directory not found, creating downloads directory")
	}
	err = os.Mkdir(processedPath, 0750)
	if err != nil && !os.IsExist(err) {
		return err
	} else if err == nil {
		slog.Info("processed directory not found, creating processed directory")
	}
	err = os.Mkdir(transcriptsPath, 0750)
	if err != nil && !os.IsExist(err) {
		return err
	} else if err == nil {
		slog.Info("transcripts directory not found, creating transcripts directory")
	}
	return nil
}

func gatherVideos(url string, isUpdate bool, safeVideoDataCollection *SafeVideoDataCollection) error {
	slog.Info("Checking channel for new videos")
	cmdFetch := exec.Command("yt-dlp", "--flat-playlist", "--print", "%(id)s", url)
	out, err := cmdFetch.CombinedOutput()
	outString := string(out)
	if err != nil {
		return errors.New(err.Error() + outString)
	}
	if isUpdate {
		slog.Info("Setting videos to be reindexed")
		addAndUpdateVideosInQueue(outString, safeVideoDataCollection)
	} else {
		addNewVideosToQueue(outString, safeVideoDataCollection)
	}
	return nil
}

func addNewVideosToQueue(videosList string, safeVideoDataCollection *SafeVideoDataCollection) {
	var wg sync.WaitGroup
	// limit goroutines to 100 at any instant to avoid consuming too much cpu and ram
	semaphore := make(chan struct{}, 100)
	var count int
	for videoId := range strings.SplitSeq(videosList, "\n") {
		if videoId == "" {
			continue
		}

		// if video details have already been recorded, skip
		videoEntry, ok := safeVideoDataCollection.Read(videoId)
		if ok {
			continue
		}

		wg.Add(1)
		go func() {
			semaphore <- struct{}{}
			defer wg.Done()
			videoDetails := getVideoDetails(videoId)
			<-semaphore
			videoEntry.Status = "pending"
			videoEntry.ReIndex = false
			videoEntry.Title = videoDetails.Title
			videoEntry.Id = videoDetails.Id
			videoEntry.UploadDate = videoDetails.UploadDate
			videoEntry.Duration = videoDetails.Duration
			safeVideoDataCollection.Write(videoId, videoEntry)
		}()

		count++
	}
	wg.Wait()
	slog.Info(fmt.Sprintf("%v new videos have been added to the queue and are pending download", count))
}

func addAndUpdateVideosInQueue(videosList string, safeVideoDataCollection *SafeVideoDataCollection) {
	var wg sync.WaitGroup
	// limit goroutines to 100 at any instant to avoid consuming too much cpu and ram
	semaphore := make(chan struct{}, 100)
	var countNew int
	var countUpdated int
	for videoId := range strings.SplitSeq(videosList, "\n") {
		if videoId == "" {
			continue
		}

		// if video details have already been recorded, skip
		videoEntry, ok := safeVideoDataCollection.Read(videoId)

		// if video details have already been recorded, update the details
		// and set it to be re-indexed while preserving its original status
		wg.Add(1)
		if ok {
			go func() {
				semaphore <- struct{}{}
				defer wg.Done()
				videoDetails := getVideoDetails(videoId)
				<-semaphore
				videoEntry.ReIndex = true
				videoEntry.Title = videoDetails.Title
				videoEntry.Id = videoDetails.Id
				videoEntry.UploadDate = videoDetails.UploadDate
				videoEntry.Duration = videoDetails.Duration
				safeVideoDataCollection.Write(videoId, videoEntry)
			}()
			countUpdated++
		} else {
			go func() {
				semaphore <- struct{}{}
				defer wg.Done()
				videoDetails := getVideoDetails(videoId)
				<-semaphore
				videoEntry.Status = "pending"
				videoEntry.ReIndex = false
				videoEntry.Title = videoDetails.Title
				videoEntry.Id = videoDetails.Id
				videoEntry.UploadDate = videoDetails.UploadDate
				videoEntry.Duration = videoDetails.Duration
				safeVideoDataCollection.Write(videoId, videoEntry)
			}()
			countNew++
		}

	}
	wg.Wait()
	slog.Info(fmt.Sprintf("%v new videos have been added to the queue and are pending download, %v video details have been updated", countNew, countUpdated))
}

func getVideoDetails(videoId string) VideoDetails {
	videoUrl := "https://www.youtube.com/watch?v=" + videoId
	// title has to be last because title has spaces within it and we use space to separate
	// this string later
	cmdFetch := exec.Command("yt-dlp", "--print", "%(upload_date)s %(duration)s %(title)s", videoUrl)
	out, err := cmdFetch.CombinedOutput()
	outString := string(out)
	if err != nil {
		slog.Warn(fmt.Sprintf("Unable to get metadata for %s: %s %s", videoId, err.Error(), outString))
		return VideoDetails{}
	}
	videoDetailsSlice := strings.SplitN(outString, " ", 3)
	fmt.Println(VideoDetails{
		Id:         videoId,
		Title:      videoDetailsSlice[2],
		UploadDate: videoDetailsSlice[0],
		Duration:   videoDetailsSlice[1],
	})

	return VideoDetails{
		Id:         videoId,
		Title:      videoDetailsSlice[2],
		UploadDate: videoDetailsSlice[0],
		Duration:   videoDetailsSlice[1],
	}

}

func downloadVideo(videoId string, safeVideoDataCollection *SafeVideoDataCollection, ouputPath string) {
	slog.Info(fmt.Sprintf("Downloading video %s", videoId))
	videoUrl := "https://www.youtube.com/watch?v=" + videoId
	// downloads audio only and saves it to the output path with name as videoId.m4a
	cmdFetch := exec.Command("yt-dlp", "-x", "--audio-format", "mp3", "-P", ouputPath, "-o", "%(id)s.%(ext)s", videoUrl)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to download video %s: %s", videoId, err.Error()+string(out)))
	} else {
		slog.Info(fmt.Sprintf("Downloaded video %s", videoId))
		videoEntry, _ := safeVideoDataCollection.Read(videoId)
		videoEntry.Status = "downloaded"
		safeVideoDataCollection.Write(videoId, videoEntry)
	}

}

func processVideo(videoId string, inputPath string, outputPath string, safeVideoDataCollection *SafeVideoDataCollection) {
	slog.Info(fmt.Sprintf("Processing video %s", videoId))
	inputFilePath := filepath.Join(inputPath, fmt.Sprintf("%s.mp3", videoId))
	outputFilePath := filepath.Join(outputPath, fmt.Sprintf("%s.wav", videoId))

	cmdFetch := exec.Command("ffmpeg", "-i", inputFilePath, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", outputFilePath)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to process video %s: %s", videoId, err.Error()+string(out)))
	} else {
		slog.Info(fmt.Sprintf("Processed video %s", videoId))
		videoEntry, _ := safeVideoDataCollection.Read(videoId)
		videoEntry.Status = "processed"
		safeVideoDataCollection.Write(videoId, videoEntry)
	}

}

func transcribeVideo(videoId string, inputPath string, outputPath string, modelPath string, safeVideoDataCollection *SafeVideoDataCollection) {
	slog.Info(fmt.Sprintf("Transcribing video %s", videoId))
	inputFilePath := filepath.Join(inputPath, fmt.Sprintf("%s.wav", videoId))
	outputFilePath := filepath.Join(outputPath, videoId)

	cmdFetch := exec.Command("whisper-cli", "-osrt", "-m", modelPath, "-f", inputFilePath, "-of", outputFilePath)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to transcribe video %s: %s", videoId, err.Error()+string(out)))
	} else {
		slog.Info(fmt.Sprintf("Transcribed video %s", videoId))
		videoEntry, _ := safeVideoDataCollection.Read(videoId)
		videoEntry.Status = "transcribed"
		safeVideoDataCollection.Write(videoId, videoEntry)
	}

}

func uploadDocumentsToMeilisearch(documents []Document, searchClient meilisearch.ServiceManager, safeVideoDataCollection *SafeVideoDataCollection) {
	slog.Info(fmt.Sprintf("Uploading %v documents to search index", len(documents)))
	_, err := searchClient.Index("videos").UpdateDocuments(documents)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to upload to index: %s", err.Error()))
	} else {
		slog.Info(fmt.Sprintf("Uploaded %v documents to search index", len(documents)))
		for _, document := range documents {
			videoEntry, _ := safeVideoDataCollection.Read(document.Id)
			videoEntry.Status = "indexed"
			videoEntry.ReIndex = false
			safeVideoDataCollection.Write(document.Id, videoEntry)

		}
	}
}

func saveProgress(projectPath string, safeVideoDataCollection *SafeVideoDataCollection) {
	updatedProgressData, err := json.MarshalIndent(safeVideoDataCollection.videosDataAndStatus, "", "\t")
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to marshall videos.json data: %v", err.Error()))
		os.Exit(1)
	}

	err = os.WriteFile(filepath.Join(projectPath, "videos.json"), updatedProgressData, 0666)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to write to videos.json: %v", err.Error()))
		os.Exit(1)
	}
}

func downloadWorker(downloadQueue <-chan string, processQueue chan<- string, outputPath string, safeVideoDataCollection *SafeVideoDataCollection) {
	for job := range downloadQueue {
		downloadVideo(job, safeVideoDataCollection, outputPath)
		processQueue <- job
	}
}

func processWorker(processQueue <-chan string, transcribeQueue chan<- string, inputPath string, outputPath string, safeVideoDataCollection *SafeVideoDataCollection) {
	for job := range processQueue {
		processVideo(job, inputPath, outputPath, safeVideoDataCollection)
		// remove file in previous step to save disk space
		downloadedFileMp3 := filepath.Join(inputPath, fmt.Sprintf("%s.mp3", job))
		downloadedFileM4a := filepath.Join(inputPath, fmt.Sprintf("%s.m4a", job))
		downloadedFileWebm := filepath.Join(inputPath, fmt.Sprintf("%s.webm", job))
		os.Remove(downloadedFileMp3)
		os.Remove(downloadedFileM4a)
		os.Remove(downloadedFileWebm)
		transcribeQueue <- job
	}
}

func transcribeWorker(transcribeQueue <-chan string, indexQueue chan<- string, inputPath string, outputPath string, modelPath string, safeVideoDataCollection *SafeVideoDataCollection, wg *sync.WaitGroup) {
	for job := range transcribeQueue {
		transcribeVideo(job, inputPath, outputPath, modelPath, safeVideoDataCollection)
		// remove file in previous step to save disk space
		processedFile := filepath.Join(inputPath, fmt.Sprintf("%s.wav", job))
		os.Remove(processedFile)
		indexQueue <- job
	}
}

func indexWorker(indexQueue <-chan string, transcriptsPath string, searchClient meilisearch.ServiceManager, safeVideoDataCollection *SafeVideoDataCollection, wg *sync.WaitGroup) {
	limiter := time.Tick(5 * time.Second)
	var documents []Document
	for {
		select {
		case job := <-indexQueue:
			videoEntry, _ := safeVideoDataCollection.Read(job)
			transcriptFilePath := filepath.Join(transcriptsPath, fmt.Sprintf("%s.srt", job))
			transcriptBytes, err := os.ReadFile(transcriptFilePath)
			if err != nil {
				slog.Error(fmt.Sprintf("Unable to read srt file: %s", err.Error()))
				continue
			}
			document := Document{
				Transcript: string(transcriptBytes),
			}
			document.Id = videoEntry.Id
			document.Title = videoEntry.Title
			document.UploadDate = videoEntry.UploadDate
			document.Duration = videoEntry.Duration
			documents = append(documents, document)
		case <-limiter:
			if len(documents) == 0 {
				continue
			}
			uploadDocumentsToMeilisearch(documents, searchClient, safeVideoDataCollection)
			// only call wg.Done() on the last step
			// because all of the jobs that have completed the last step
			// will be the sum of all the jobs input to all the pipelines
			for range len(documents) {
				wg.Done()
			}
			documents = nil
		}
	}
}

func printSummary(safeVideoDataCollection *SafeVideoDataCollection) {
	countTotal := len(safeVideoDataCollection.videosDataAndStatus)
	var countPending int
	var countDownloaded int
	var countProcessed int
	var countTranscribed int
	var countIndexed int
	var countReindex int

	for _, video := range safeVideoDataCollection.videosDataAndStatus {
		switch video.Status {
		case "pending":
			countPending++
		case "downloaded":
			countDownloaded++
		case "processed":
			countProcessed++
		case "transcribed":
			countTranscribed++
		case "indexed":
			countIndexed++
		default:

		}

		if video.ReIndex && video.Status == "indexed" {
			countReindex++
		}
	}

	slog.Info(fmt.Sprintf(`========== Summary: ==========

Enqueued a total of %v videos
Indexed %v videos

Pending Download: %v
Pending Processing: %v
Pending Transcribing: %v
Pending Indexing: %v
Pending Re-Indexing: %v


`, countTotal,
		countIndexed,
		countPending,
		countDownloaded,
		countProcessed,
		countTranscribed,
		countReindex,
	))
}
