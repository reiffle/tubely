package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

func getVideoAspectRatio(filepath string) (string, error) {
	//Set the filepath to the video file you want to analyze
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filepath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running ffprobe command: %w", err)
	}
	aspectRatio, err := getRatioFromOutput(stdout)
	if err != nil {
		return "", fmt.Errorf("error getting aspect ratio: %w", err)
	}
	return aspectRatio, nil
}

func getRatioFromOutput(output bytes.Buffer) (string, error) {
	type Stream struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}

	type streams struct {
		Streams []Stream `json:"streams"`
	}

	var s streams
	err := json.Unmarshal(output.Bytes(), &s)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling ffprobe output: %w", err)
	}
	if len(s.Streams) == 0 {
		return "", fmt.Errorf("no streams found in video file")
	}

	width := s.Streams[0].Width
	height := s.Streams[0].Height

	if height == 0 || width == 0 {
		return "", fmt.Errorf("cannot calculate aspect ratio")
	}
	return calcRatio(width, height), nil
}

func calcRatio(width, height int) string {
	// Calculate the aspect ratio
	bigNum := 16.0
	littleNum := 9.0
	tolerance := 0.01
	trueRatio := float64(width) / float64(height)
	if trueRatio > (bigNum/littleNum)*(1.0-tolerance) && trueRatio < (bigNum/littleNum)*(1.0+tolerance) {
		return "16:9"
	} else if trueRatio > (littleNum/bigNum)*(1.0-tolerance) && trueRatio < (littleNum/bigNum)*(1.0+tolerance) {
		return "9:16"
	} else {
		return "other"
	}
}
