# Youtube to Meilisearch (YTMS) Helper tool
Automate the transcription of any Channel's YouTube videos using AI and upload the transcripts to a Meilisearch instance.

YTMS makes use of yt-dlp to scan a YouTube Channel for videos. Then YTMS downloads each video from that channel, transcribes them using whisper.cpp and produces srt files of the transcripts. Finally, the transcripts along with video details (title, upload date and duration) are uploaded as json data to the configured Meilisearch instance. The downloading, processing, transcription and uploading of video data are done in parallel for maximum speed. The extent of parellelization can be configured further to increase speed of the entire process. Progress is saved automatically and YTMS will resume from where it left of when launched again.



## Prerequisites
- [yt-dlp](github.com/yt-dlp/yt-dlp) installed to $PATH
- whisper-cli from [whisper.cpp](https://github.com/ggml-org/whisper.cpp) installed to $PATH
- ffmpeg installed to $PATH
- [Meilisearch](https://www.meilisearch.com/) instance (either self-hosted or cloud) - YTMS still works without Meilisearch, in which case it will simple produce .srt transcripts and will fail at the uploading step without interrupting the rest of the process.

## Usage Instructions

### Build
1. Clone this repo
2. Run `go build` inside the repo directory

### Configure
1. Create .env file and set env variables. Refer to .env.example

The following env variables have to be set up for the tool to work.
 - `DATA_PATH` - This is where all the transcripts will be saved and also the save progress of YTMS. Videos that are being downloaded and processed will also be stored in this directory, and will be cleaned up automatically. Choose a directory that you have write permissions to
 - `CHANNEL_URL` - The URL of the YouTube channel from which the videos will be transcribed
 - `MEILISEARCH_URL` - The URL of the Meilisearch Instance. If video transcripts do not need to be uploaded to Meilisearch, this can be left blank
 - `MEILISEARCH_API_KEY` - The API Key of the Meilisearch Instance. If video transcripts do not need to be uploaded to Meilisearch, this can be left blank
 - `WHISPER_MODEL_PATH` - File Path to the whisper model that will be used for transcription. Refer to Whisper.cpp documentation for details

> [!warning]
> Set the below values responsibily. Setting them too high can cause the system to run out of resources and crash
 - `MAX_DOWNLOAD_PROCESS_WORKERS` - The number of download workers and process workers that will be run in parallel. A value of two will run two yt-dlp processes and two ffmpeg processes in parallel. It is recommended to set this to n + 1 where n is the number of Transcribe workers. This ensures that a video is always available to be transcribed by the transcribe worker.
 - `MAX_VIDEO_DETAIL_FETCH_WORKERS` - The number of yt-dlp processes that will be run in parallel to fetch video details such as title, upload date and duration of video. It is recommended to set this between 10-20. Higher values can be used if more system resources are available.
 - `MAX_TRANSCRIBE_WORKERS` - The number of whisper.cpp processes that will run in parallel to transcribe videos. It is recommended to set this to 1 and monitor system resouces first, then experiment with increasing it while keeping an eye on system resources used. Higher values can be used if using GPU with a high VRAM to run the Whisper model.

### Run

Run the tool `./yt-meilisearch-helper` from within the repo directory

## Contributing
Contributions are welcome. Please fork the repo and open pull requests to contribute.

## License
This tool is released under the MIT License
