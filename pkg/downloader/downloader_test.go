package downloader

import (
	"context"
	"os"
	"testing"
)

func TestChunkAllocationAndRangeCoverage(t *testing.T) {
	totalSize := int64(1000)
	numChunks := 4
	job := NewDownloadJob("http://example.com/test", "/tmp/test.tmp", totalSize, numChunks, nil)

	if len(job.Chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(job.Chunks))
	}

	if job.Chunks[0].StartByte != 0 || job.Chunks[0].EndByte != 249 {
		t.Errorf("chunk 0 range mismatch: %d-%d", job.Chunks[0].StartByte, job.Chunks[0].EndByte)
	}

	if job.Chunks[3].StartByte != 750 || job.Chunks[3].EndByte != 999 {
		t.Errorf("chunk 3 range mismatch: %d-%d", job.Chunks[3].StartByte, job.Chunks[3].EndByte)
	}

	// Test simulation execution
	ctx := context.Background()
	job.Simulate(ctx, 2, nil)

	downloaded, total, percent := job.GetProgress()
	if downloaded != totalSize || percent != 100.0 {
		t.Errorf("expected 100%% progress, got %.2f%% (%d/%d)", percent, downloaded, total)
	}

	for i, c := range job.Chunks {
		if c.State != StateCompleted {
			t.Errorf("chunk %d expected COMPLETED state, got %s", i, c.State)
		}
	}
}

func TestFileTruncateAndWriteAt(t *testing.T) {
	tmpFile := "/tmp/test_truncate.tmp"
	defer os.Remove(tmpFile)

	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer f.Close()

	if err := f.Truncate(500); err != nil {
		t.Fatalf("failed to truncate: %v", err)
	}

	data := []byte("hello offset")
	if _, err := f.WriteAt(data, 100); err != nil {
		t.Fatalf("failed to WriteAt: %v", err)
	}

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if fi.Size() != 500 {
		t.Errorf("expected file size 500, got %d", fi.Size())
	}
}
