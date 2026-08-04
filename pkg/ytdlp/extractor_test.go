package ytdlp

import (
	"encoding/json"
	"testing"
)

func TestParseVideoInfo(t *testing.T) {
	rawJSON := `{
		"id": "sample123",
		"title": "Test Video Title",
		"ext": "mp4",
		"formats": [
			{
				"format_id": "137",
				"url": "https://example.com/video.mp4",
				"ext": "mp4",
				"vcodec": "avc1.64001e",
				"acodec": "none",
				"filesize": 1048576,
				"protocol": "https",
				"height": 1080,
				"tbr": 2500,
				"http_headers": {
					"User-Agent": "TestAgent"
				}
			},
			{
				"format_id": "140",
				"url": "https://example.com/audio.m4a",
				"ext": "m4a",
				"vcodec": "none",
				"acodec": "mp4a.40.2",
				"filesize": 262144,
				"protocol": "https",
				"http_headers": {
					"User-Agent": "TestAgent"
				}
			}
		]
	}`

	var info VideoInfo
	err := json.Unmarshal([]byte(rawJSON), &info)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if info.ID != "sample123" {
		t.Errorf("expected ID 'sample123', got '%s'", info.ID)
	}

	bestVideo := info.BestVideoFormat()
	if bestVideo == nil || bestVideo.FormatID != "137" {
		t.Errorf("expected best video format '137', got %v", bestVideo)
	}

	bestAudio := info.BestAudioFormat()
	if bestAudio == nil || bestAudio.FormatID != "140" {
		t.Errorf("expected best audio format '140', got %v", bestAudio)
	}
}

func TestCodecPreferenceAV1OverH264(t *testing.T) {
	rawJSON := `{
		"id": "sample456",
		"title": "Codec Comparison",
		"formats": [
			{
				"format_id": "h264_1080p",
				"height": 1080,
				"vcodec": "avc1.64002a",
				"tbr": 3500.0
			},
			{
				"format_id": "vp9_1080p",
				"height": 1080,
				"vcodec": "vp09.00.40.08",
				"tbr": 2200.0
			},
			{
				"format_id": "av1_1080p",
				"height": 1080,
				"vcodec": "av01.0.08M.08",
				"tbr": 1500.0
			}
		]
	}`

	var info VideoInfo
	if err := json.Unmarshal([]byte(rawJSON), &info); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	best := info.BestVideoFormat()
	if best == nil {
		t.Fatal("expected non-nil format")
	}

	if best.FormatID != "av1_1080p" {
		t.Errorf("expected AV1 format 'av1_1080p' for optimal size/quality balance, got '%s'", best.FormatID)
	}
}
