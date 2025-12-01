package main

import "testing"

func TestVideoAspectRatio(t *testing.T) {
	filePath := "samples/boots-video-horizontal.mp4"
	res, err := getVideoAspectRatio(filePath)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if res != "16:9" {
		t.Errorf("Expected 16:9, but got %v", res)
	}
}
