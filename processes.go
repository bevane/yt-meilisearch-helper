package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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

func gatherVideos(url string, videosDataAndStatus VideosDataAndStatus) error {
	slog.Info("Checking channel for new videos")
	cmdFetch := exec.Command("yt-dlp", "--flat-playlist", "--print", "%(id)s %(title)s", url)
	out, err := cmdFetch.CombinedOutput()
	outString := string(out)
	if err != nil {
		return errors.New(err.Error() + outString)
	}
	addNewVideosToProcessing(outString, videosDataAndStatus)
	return nil
}

func addNewVideosToProcessing(videosList string, videosDataAndStatus VideosDataAndStatus) {
	var count int
	for videoIdAndTitle := range strings.SplitSeq(videosList, "\n") {
		if videoIdAndTitle == "" {
			continue
		}
		videoIdAndTitleSlice := strings.SplitN(videoIdAndTitle, " ", 2)
		videoId, title := videoIdAndTitleSlice[0], videoIdAndTitleSlice[1]

		// if video details have already been recorded, skip
		videoEntry, ok := videosDataAndStatus[videoId]
		if ok {
			continue
		}

		videoEntry.Status = "pending"
		videoEntry.Title = title
		videoEntry.Id = videoId
		videosDataAndStatus[videoId] = videoEntry

		count++
	}
	slog.Info(fmt.Sprintf("%v new videos have been added to the queue and are pending download", count))
}

func downloadVideo(videoId string, videosDataAndStatus VideosDataAndStatus, ouputPath string) {
	slog.Info(fmt.Sprintf("Downloading video %s", videoId))
	videoUrl := "https://www.youtube.com/watch?v=" + videoId
	// downloads audio only and saves it to the output path with name as videoId.m4a
	cmdFetch := exec.Command("yt-dlp", "-x", "--audio-format", "mp3", "-P", ouputPath, "-o", "%(id)s.%(ext)s", videoUrl)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to download video %s: %s", videoId, err.Error()+string(out)))
	} else {
		slog.Info(fmt.Sprintf("Downloaded video %s", videoId))
		videoEntry, _ := videosDataAndStatus[videoId]
		videoEntry.Status = "downloaded"
		videosDataAndStatus[videoId] = videoEntry
	}

}

func processVideo(videoId string, inputPath string, outputPath string, videosDataAndStatus VideosDataAndStatus) {
	slog.Info(fmt.Sprintf("Processing video %s", videoId))
	inputFilePath := filepath.Join(inputPath, fmt.Sprintf("%s.mp3", videoId))
	outputFilePath := filepath.Join(outputPath, fmt.Sprintf("%s.wav", videoId))

	cmdFetch := exec.Command("ffmpeg", "-i", inputFilePath, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", outputFilePath)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to process video %s: %s", videoId, err.Error()+string(out)))
	} else {
		slog.Info(fmt.Sprintf("Processed video %s", videoId))
		videoEntry, _ := videosDataAndStatus[videoId]
		videoEntry.Status = "processed"
		videosDataAndStatus[videoId] = videoEntry
	}

}

func transcribeVideo(videoId string, inputPath string, outputPath string, modelPath string, videosDataAndStatus VideosDataAndStatus) {
	slog.Info(fmt.Sprintf("Transcribing video %s", videoId))
	inputFilePath := filepath.Join(inputPath, fmt.Sprintf("%s.wav", videoId))
	outputFilePath := filepath.Join(outputPath, videoId)

	cmdFetch := exec.Command("whisper-cli", "-osrt", "-m", modelPath, "-f", inputFilePath, "-of", outputFilePath)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to transcribe video %s: %s", videoId, err.Error()+string(out)))
	} else {
		slog.Info(fmt.Sprintf("Transcribed video %s", videoId))
		videoEntry, _ := videosDataAndStatus[videoId]
		videoEntry.Status = "transcribed"
		videosDataAndStatus[videoId] = videoEntry
	}

}

func uploadDocumentsToMeilisearch(documents []Document, searchClient meilisearch.ServiceManager, videosDataAndStatus VideosDataAndStatus) {
	slog.Info(fmt.Sprintf("Uploading %v documents to search index", len(documents)))
	_, err := searchClient.Index("videos").UpdateDocuments(documents)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to upload to index: %s", err.Error()))
	} else {
		slog.Info(fmt.Sprintf("Uploaded %v documents to search index", len(documents)))
		for _, document := range documents {
			videoEntry, _ := videosDataAndStatus[document.Id]
			videoEntry.Status = "indexed"
			videosDataAndStatus[document.Id] = videoEntry

		}
	}
}

func cleanup(root string) {
	slog.Info("Cleaning up and exiting")
	fsys := os.DirFS(root)
	fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn(fmt.Sprintf("Error accessing path %s: %v\n", path, err))
			return nil // Continue walking despite the error
		}

		if !d.IsDir() {
			filename := filepath.Base(path)
			if strings.HasSuffix(filename, ".cleanup") || strings.HasSuffix(filename, ".cleanup2") {
				fullPath := filepath.Join(root, path)
				err := os.Remove(fullPath)
				if err != nil {
					slog.Warn(fmt.Sprintf("Error removing file %s: %v\n", path, err))
					return nil // Continue walking despite the error
				}
				fmt.Printf("Cleanup: removed file: %s\n", fullPath)
			}
		}
		return nil
	})
}

func saveProgress(projectPath string, videosDataAndStatus VideosDataAndStatus) {
	updatedProgressData, err := json.MarshalIndent(videosDataAndStatus, "", "\t")
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

func downloadWorker(downloadQueue <-chan string, processQueue chan<- string, outputPath string, progress VideosDataAndStatus) {
	for job := range downloadQueue {
		downloadVideo(job, progress, outputPath)
		processQueue <- job
	}
}

func processWorker(processQueue <-chan string, transcribeQueue chan<- string, inputPath string, outputPath string, progress VideosDataAndStatus) {
	for job := range processQueue {
		processVideo(job, inputPath, outputPath, progress)
		transcribeQueue <- job
	}
}

func transcribeWorker(transcribeQueue <-chan string, inputPath string, outputPath string, modelPath string, progress VideosDataAndStatus, wg *sync.WaitGroup) {
	for job := range transcribeQueue {
		transcribeVideo(job, inputPath, outputPath, modelPath, progress)
		// only call wg.Done() on the last step
		// because all of the jobs that have completed the last step
		// will be the sum of all the jobs input to all the pipelines
		wg.Done()
	}
}

func indexWorker(indexQueue <-chan string, transcriptsPath string, searchClient meilisearch.ServiceManager, progress VideosDataAndStatus, wg *sync.WaitGroup) {
	limiter := time.Tick(5 * time.Second)
	var documents []Document
	for {
		select {
		case job := <-indexQueue:
			videoEntry, _ := progress[job]
			transcriptFilePath := filepath.Join(transcriptsPath, fmt.Sprintf("%s.srt", job))
			transcriptBytes, err := os.ReadFile(transcriptFilePath)
			if err != nil {
				slog.Error(fmt.Sprintf("Unable to read srt file: %s", err.Error()))
				continue
			}
			document := Document{
				Id:         videoEntry.Id,
				Title:      videoEntry.Title,
				Transcript: string(transcriptBytes),
			}
			documents = append(documents, document)
		case <-limiter:
			uploadDocumentsToMeilisearch(documents, searchClient, progress)
			for range len(documents) {
				wg.Done()
			}
			documents = nil
		}
	}
}
