package main

import (
	"os"

	"github.com/joho/godotenv"
)

type VideoProcessingStatus map[string]string

func main() {
	godotenv.Load(".env")
	dataPath := os.Getenv("DATA_PATH")
	initDataDir(dataPath)
}
