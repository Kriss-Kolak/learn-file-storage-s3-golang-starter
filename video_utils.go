package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
)

func getVideoAspectRatio(filePath string) (string, error) {

	type parameters struct {
		Streams []struct {
			Height float64 `json:"height"`
			Width  float64 `json:"width"`
		}
	}

	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)

	newBuff := bytes.Buffer{}
	cmd.Stdout = &newBuff
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	params := parameters{}
	data := newBuff.Bytes()
	err = json.Unmarshal(data, &params)
	if err != nil {
		return "", err
	}

	if len(params.Streams) == 0 {
		return "other", nil
	}

	ratio := params.Streams[0].Width / params.Streams[0].Height
	var tolerance float64 = 0.01
	var value_16_9 float64 = float64(16) / float64(9)
	var value_9_16 float64 = float64(9) / float64(16)

	if ratio >= value_16_9-value_16_9*tolerance && ratio <= value_16_9+value_16_9*tolerance {
		return "16:9", nil
	}
	if ratio >= value_9_16-value_9_16*tolerance && ratio <= value_9_16+value_9_16*tolerance {
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"
	cmd := exec.Command("ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags", "faststart",
		"-f", "mp4",
		outputFilePath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outputFilePath, nil
}
