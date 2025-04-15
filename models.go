package main

import "sync"

// when adding new fields, the gatherVideos and IndexWorker functions
// have to be updated to assign values to the new fields
type VideoDetails struct {
	Id         string `json:"id"`
	Title      string `json:"title"`
	UploadDate string `json:"uploadDate"`
	Duration   string `json:"duration"`
}

type Document struct {
	Transcript string `json:"transcript"`
	VideoDetails
}

type VideoData struct {
	Status  string `json:"status"`
	ReIndex bool   `json:"reIndex"`
	VideoDetails
}

type VideoDataCollection map[string]VideoData

type SafeVideoDataCollection struct {
	videosDataAndStatus VideoDataCollection
	mu                  sync.Mutex
}

func (sv *SafeVideoDataCollection) Read(videoId string) (VideoData, bool) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	videoEntry, ok := sv.videosDataAndStatus[videoId]
	return videoEntry, ok
}

func (sv *SafeVideoDataCollection) Write(videoId string, data VideoData) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.videosDataAndStatus[videoId] = data
}
