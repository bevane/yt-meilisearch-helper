package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func initDataDir(dataPath string) error {
	progressPath := filepath.Join(dataPath, "progress.json")
	indexPath := filepath.Join(dataPath, "videos.json")
	downloadsPath := filepath.Join(dataPath, "downloads")
	processedPath := filepath.Join(dataPath, "processed")
	transcriptsPath := filepath.Join(dataPath, "transcripts")

	// the logic for creating files and directories will be different
	// because os.Mkdir and os.Create return errors differently in case
	// the file or directory already exists
	// os.Create does not throw error if the file already exists and instead
	// will truncate the file so check if file exists explicitly with os.Stat
	_, err := os.Stat(progressPath)
	if err != nil && os.IsNotExist(err) {
		slog.Info("progress.json not found, creating progress.json")
		progressFile, err := os.Create(progressPath)
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
	_, err = os.Stat(indexPath)
	if err != nil && os.IsNotExist(err) {
		slog.Info("videos.json not found, creating videos.json")
		indexFile, err := os.Create(indexPath)
		if err != nil {
			return err
		}
		defer indexFile.Close()
		_, err = indexFile.Write([]byte("{}"))
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

func gatherVideos(url string, videoProcessingStatus VideoProcessingStatus) error {
	cmdFetch := exec.Command("yt-dlp", "--flat-playlist", "--print", "id", url)
	out, err := cmdFetch.CombinedOutput()
	if err != nil {
		return errors.New(string(out))
	}
	var count int
	for videoId := range strings.SplitSeq(string(out), "\n") {
		if _, ok := videoProcessingStatus[videoId]; ok {
			continue
		}
		if videoId == "" {
			continue
		}
		count++
		videoProcessingStatus[videoId] = "pending"
	}
	slog.Info(fmt.Sprintf("%v new videos have been added to the queue and are pending download", count))
	return nil
}
