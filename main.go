package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/tidwall/gjson"
)

type RenameCmd struct {
	VideosDirectory *string `arg:"--videos-dir" help:"path to directory containing GoPro video files"`
	CommitToRename  bool    `arg:"--commit-to-rename" help:"really rename files, not just do a dry run"`
}

var options struct {
	Rename *RenameCmd `arg:"subcommand:rename" help:"rename GoPro video files"`
}

// Regex used to match GoPro-created recording files
var recordingRegex = regexp.MustCompile("^G([XH])([0-9]{2})([0-9]{4}).(.*)")

func rename() error {

	recordingDirFiles, err := os.ReadDir(*options.Rename.VideosDirectory)
	if err != nil {
		return err
	}

	foundVideo := false
	videoTimestamps := map[int]time.Time{}

	renameMessage := "Dry run"
	if options.Rename.CommitToRename {
		renameMessage = "Renaming"
	}

	// For each file in the recording directory...
	for _, file := range recordingDirFiles {

		// Check for regex matches of video file, skip if not named according to convention
		matches := recordingRegex.FindStringSubmatch(file.Name())
		if matches == nil || len(matches) < 5 {
			fmt.Printf("File \"%s\" does not match regex. Is it named properly?\n", file.Name())
			continue
		}
		foundVideo = true

		// Video chunk part number
		videoPart, err := strconv.Atoi(matches[2])
		if err != nil {
			return err
		}

		// Video chunk's associated ID common with similar chunks
		videoId, err := strconv.Atoi(matches[3])
		if err != nil {
			return err
		}

		// File extension
		fileNameExt := strings.ToLower(matches[4])

		// Absolute path to current video chunk file
		absPathSource := path.Join(*options.Rename.VideosDirectory, file.Name())

		// If no timestamp was cached by a full video composed from current chunk, parse chunk's metadata
		if _, ok := videoTimestamps[videoId]; !ok {

			// Use ffmpeg to print current video chunk's metadata in JSON, hopefully with timestamp info
			ffmpegOutJson, err := exec.Command("ffprobe", absPathSource, "-print_format", "json", "-show_entries", "format_tags=creation_time").Output()
			if err != nil {
				return fmt.Errorf("Error parsing video metadata output from \"%s\" as JSON", absPathSource)
			}

			// Attempt to extract string representation of the current chunk's timestamp
			chunkTimestampOption := gjson.Get(string(ffmpegOutJson), "format.tags.creation_time")
			if !chunkTimestampOption.Exists() {
				return fmt.Errorf("Could not find 'creation_time' field in metadata output for %s.", file.Name())
			}
			chunkTimestampStr := chunkTimestampOption.String()

			// Parse timestamp into Go's time.Time, cache for later use
			newChunkTimestamp, err := time.Parse("2006-01-02T15:04:05.9Z", chunkTimestampStr)
			if err != nil {
				return err
			}

			videoTimestamps[videoId] = newChunkTimestamp
		}

		// Retrieve valid timestamp
		videoTimestamp := videoTimestamps[videoId]

		// Format timestamp for use in destination filename
		destFileNameTimestamp := fmt.Sprintf(
			"%04d-%02d-%02d %02d_%02d_%02d",
			videoTimestamp.Year(),
			videoTimestamp.Month(),
			videoTimestamp.Day(),
			videoTimestamp.Hour(),
			videoTimestamp.Minute(),
			videoTimestamp.Second(),
		)

		// Basename of destination file, composed of multiple sources of metadata
		destFileName := fmt.Sprintf("GoPro Recording _-_ Date %s _-_ ID %04d _-_ Part %02d.%s", destFileNameTimestamp, videoId, videoPart, fileNameExt)

		// Absolute path of destination file
		absPathDest := path.Join(*options.Rename.VideosDirectory, destFileName)

		// Skip if destination file already exists
		if _, err := os.Stat(absPathDest); !errors.Is(err, os.ErrNotExist) {
			fmt.Printf("File \"%s\" already exists, skipping...\n", destFileName)
			continue
		}

		// Message user about renaming action
		fmt.Printf("%s: %s -> %s\n", renameMessage, absPathSource, absPathDest)

		// If user explicitly set flag to rename, then follow through
		if options.Rename.CommitToRename {
			if err := os.Rename(absPathSource, absPathDest); err != nil {
				return err
			}

		}

	}

	if !foundVideo {
		return fmt.Errorf("Directory does not contain GoPro videos. Are the files named according to convention?")
	}

	return nil
}

func main() {

	// Parse options
	arg.MustParse(&options)

	if options.Rename.VideosDirectory == nil {
		fmt.Fprintln(os.Stderr, "Did not specify video directory")
	}

	if err := rename(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

}
