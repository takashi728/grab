package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type ChunkState int

const (
	StatePending ChunkState = iota
	StateDownloading
	StateCompleted
	StateFailed
)

func (s ChunkState) String() string {
	switch s {
	case StatePending:
		return "PENDING"
	case StateDownloading:
		return "DOWNLOADING"
	case StateCompleted:
		return "COMPLETED"
	case StateFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

type Chunk struct {
	Index          int        `json:"index"`
	StartByte      int64      `json:"start_byte"`
	EndByte        int64      `json:"end_byte"`
	Downloaded     int64      `json:"downloaded"`
	TotalBytes     int64      `json:"total_bytes"`
	State          ChunkState `json:"state"`
	WorkerID       int        `json:"worker_id"`
	LastError      error      `json:"-"`
}

type DownloadJob struct {
	URL            string            `json:"url"`
	OutputPath     string            `json:"output_path"`
	TotalSize      int64             `json:"total_size"`
	NumChunks      int               `json:"num_chunks"`
	Headers        map[string]string `json:"headers"`
	Chunks         []*Chunk          `json:"chunks"`
	TotalDownloaded int64            `json:"total_downloaded"`
	mu             sync.Mutex
}

func NewDownloadJob(url, outputPath string, totalSize int64, numChunks int, headers map[string]string) *DownloadJob {
	if numChunks <= 0 {
		numChunks = 4
	}
	job := &DownloadJob{
		URL:        url,
		OutputPath: outputPath,
		TotalSize:  totalSize,
		NumChunks:  numChunks,
		Headers:    headers,
		Chunks:     make([]*Chunk, numChunks),
	}

	chunkSize := totalSize / int64(numChunks)
	var start int64 = 0

	for i := 0; i < numChunks; i++ {
		end := start + chunkSize - 1
		if i == numChunks-1 {
			end = totalSize - 1
		}
		job.Chunks[i] = &Chunk{
			Index:      i,
			StartByte:  start,
			EndByte:    end,
			Downloaded: 0,
			TotalBytes: end - start + 1,
			State:      StatePending,
		}
		start = end + 1
	}

	return job
}

func (j *DownloadJob) GetProgress() (downloaded int64, total int64, percent float64) {
	j.mu.Lock()
	defer j.mu.Unlock()

	var sum int64 = 0
	for _, c := range j.Chunks {
		sum += c.Downloaded
	}
	j.TotalDownloaded = sum
	total = j.TotalSize
	if total > 0 {
		percent = (float64(sum) / float64(total)) * 100.0
	}
	return sum, total, percent
}

// Execute performs real HTTP range downloads across parallel workers into file
func (j *DownloadJob) Execute(ctx context.Context, concurrency int, client *http.Client, onUpdate func()) error {
	if client == nil {
		client = http.DefaultClient
	}

	// Pre-allocate output file
	outFile, err := os.OpenFile(j.OutputPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if j.TotalSize > 0 {
		if err := outFile.Truncate(j.TotalSize); err != nil {
			return fmt.Errorf("failed to truncate file to size %d: %w", j.TotalSize, err)
		}
	}

	workChan := make(chan *Chunk, len(j.Chunks))
	for _, c := range j.Chunks {
		workChan <- c
	}
	close(workChan)

	var wg sync.WaitGroup
	errChan := make(chan error, concurrency)

	for workerID := 1; workerID <= concurrency; workerID++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			for chunk := range workChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if err := j.downloadChunk(ctx, client, outFile, chunk, wID, onUpdate); err != nil {
					errChan <- fmt.Errorf("worker %d chunk %d error: %w", wID, chunk.Index, err)
					return
				}
			}
		}(workerID)
	}

	wg.Wait()
	close(errChan)

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

func (j *DownloadJob) downloadChunk(ctx context.Context, client *http.Client, outFile *os.File, chunk *Chunk, workerID int, onUpdate func()) error {
	j.mu.Lock()
	chunk.State = StateDownloading
	chunk.WorkerID = workerID
	j.mu.Unlock()
	if onUpdate != nil {
		onUpdate()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", j.URL, nil)
	if err != nil {
		j.mu.Lock()
		chunk.State = StateFailed
		chunk.LastError = err
		j.mu.Unlock()
		return err
	}

	// Forward headers
	for k, v := range j.Headers {
		req.Header.Set(k, v)
	}

	// Set byte range header
	rangeHeader := fmt.Sprintf("bytes=%d-%d", chunk.StartByte+chunk.Downloaded, chunk.EndByte)
	req.Header.Set("Range", rangeHeader)

	resp, err := client.Do(req)
	if err != nil {
		j.mu.Lock()
		chunk.State = StateFailed
		chunk.LastError = err
		j.mu.Unlock()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		j.mu.Lock()
		chunk.State = StateFailed
		chunk.LastError = err
		j.mu.Unlock()
		return err
	}

	buf := make([]byte, 32*1024)
	offset := chunk.StartByte + chunk.Downloaded

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			// Seek & Write at specific chunk offset safely
			_, wErr := outFile.WriteAt(buf[:n], offset)
			if wErr != nil {
				j.mu.Lock()
				chunk.State = StateFailed
				chunk.LastError = wErr
				j.mu.Unlock()
				return wErr
			}

			offset += int64(n)

			j.mu.Lock()
			chunk.Downloaded += int64(n)
			j.mu.Unlock()

			if onUpdate != nil {
				onUpdate()
			}
		}

		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			j.mu.Lock()
			chunk.State = StateFailed
			chunk.LastError = rErr
			j.mu.Unlock()
			return rErr
		}
	}

	j.mu.Lock()
	chunk.State = StateCompleted
	j.mu.Unlock()
	if onUpdate != nil {
		onUpdate()
	}

	return nil
}

// Simulate executes simulated chunk progress for testing the state machine UI without network
func (j *DownloadJob) Simulate(ctx context.Context, concurrency int, onUpdate func()) {
	var wg sync.WaitGroup
	workChan := make(chan *Chunk, len(j.Chunks))
	for _, c := range j.Chunks {
		workChan <- c
	}
	close(workChan)

	var activeCount int32 = 0

	for workerID := 1; workerID <= concurrency; workerID++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			for chunk := range workChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				atomic.AddInt32(&activeCount, 1)

				j.mu.Lock()
				chunk.State = StateDownloading
				chunk.WorkerID = wID
				j.mu.Unlock()
				if onUpdate != nil {
					onUpdate()
				}

				stepSize := chunk.TotalBytes / 10
				if stepSize <= 0 {
					stepSize = 1
				}

				for chunk.Downloaded < chunk.TotalBytes {
					select {
					case <-ctx.Done():
						return
					case <-time.After(50 * time.Millisecond):
					}

					j.mu.Lock()
					chunk.Downloaded += stepSize
					if chunk.Downloaded > chunk.TotalBytes {
						chunk.Downloaded = chunk.TotalBytes
					}
					j.mu.Unlock()

					if onUpdate != nil {
						onUpdate()
					}
				}

				j.mu.Lock()
				chunk.State = StateCompleted
				j.mu.Unlock()
				atomic.AddInt32(&activeCount, -1)

				if onUpdate != nil {
					onUpdate()
				}
			}
		}(workerID)
	}

	wg.Wait()
}
