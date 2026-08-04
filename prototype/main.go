// PROTOTYPE — Throwaway state model validator for grab chunk downloader engine.
// Question being answered:
// "Does the concurrent byte-range chunk downloader state machine handle dynamic worker assignment,
// byte offset assembly, status transitions (PENDING -> DOWNLOADING -> COMPLETED), and progress state cleanly?"
//
// Run command: go run ./prototype

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"grab/pkg/downloader"
)

func renderFrame(job *downloader.DownloadJob, concurrency int, isRunning bool, statusMsg string) {
	// Clear screen & set cursor to top-left
	fmt.Print("\033[2J\033[H")

	fmt.Println("\x1b[1m=======================================================================\x1b[0m")
	fmt.Println("\x1b[1m  PROTOTYPE: grab Concurrent Chunk Downloader State Machine\x1b[0m")
	fmt.Println("\x1b[2m  Question: Does the range chunk state model & worker pool render state cleanly?\x1b[0m")
	fmt.Println("\x1b[1m=======================================================================\x1b[0m")
	fmt.Println()

	downloaded, total, percent := job.GetProgress()

	fmt.Printf("\x1b[1mTarget URL:\x1b[0m %s\n", job.URL)
	fmt.Printf("\x1b[1mOutput Path:\x1b[0m %s\n", job.OutputPath)
	fmt.Printf("\x1b[1mTotal Size:\x1b[0m  %s\n", downloader.FormatBytes(total))
	fmt.Printf("\x1b[1mProgress:\x1b[0m    [%s] \x1b[1;32m%.2f%%\x1b[0m (%s / %s)\n",
		makeProgressBar(percent, 30), percent, downloader.FormatBytes(downloaded), downloader.FormatBytes(total))
	fmt.Printf("\x1b[1mConcurrency:\x1b[0m %d workers | \x1b[1mChunks:\x1b[0m %d\n", concurrency, len(job.Chunks))
	fmt.Println()

	fmt.Println("\x1b[1m--- Chunk State Matrix ---\x1b[0m")
	for _, c := range job.Chunks {
		var cPercent float64 = 0
		if c.TotalBytes > 0 {
			cPercent = (float64(c.Downloaded) / float64(c.TotalBytes)) * 100.0
		}

		stateColor := "\x1b[33m" // Yellow for pending
		switch c.State {
		case downloader.StateDownloading:
			stateColor = "\x1b[36m" // Cyan
		case downloader.StateCompleted:
			stateColor = "\x1b[32m" // Green
		case downloader.StateFailed:
			stateColor = "\x1b[31m" // Red
		}

		workerInfo := "Worker --"
		if c.WorkerID > 0 {
			workerInfo = fmt.Sprintf("Worker #%d", c.WorkerID)
		}

		fmt.Printf("Chunk #%02d [%s% -11s\x1b[0m] %s | Range: %10d - %10d | [%s] %5.1f%%\n",
			c.Index,
			stateColor, c.State.String(),
			workerInfo,
			c.StartByte, c.EndByte,
			makeProgressBar(cPercent, 15),
			cPercent,
		)
	}
	fmt.Println()

	if statusMsg != "" {
		fmt.Printf("\x1b[1;33mStatus:\x1b[0m %s\n\n", statusMsg)
	}

	fmt.Println("\x1b[1m-----------------------------------------------------------------------\x1b[0m")
	if isRunning {
		fmt.Println("\x1b[1;36m[Simulation Running...]\x1b[0m Press Ctrl+C to cancel.")
	} else {
		fmt.Println("\x1b[1mControls:\x1b[0m  [\x1b[1ms\x1b[0m] run simulation   [\x1b[1mr\x1b[0m] reset job   [\x1b[1m+\x1b[0m] add chunk   [\x1b[1m-\x1b[0m] sub chunk   [\x1b[1mq\x1b[0m] quit")
	}
}

func makeProgressBar(percent float64, width int) string {
	filled := int((percent / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">" + strings.Repeat("-", width-filled-1)
	}
	return bar
}

func main() {
	var (
		totalSize   int64 = 100 * 1024 * 1024 // 100 MB sample
		numChunks         = 8
		concurrency       = 4
		job               = downloader.NewDownloadJob("https://example.com/sample_video.mp4", "/tmp/grab_sample.mp4", totalSize, numChunks, nil)
		mu          sync.Mutex
		isRunning   bool
		statusMsg   string = "Ready to simulate chunk download."
	)

	// Render initial state
	renderFrame(job, concurrency, false, statusMsg)

	// Set terminal raw mode / non-blocking keystroke reading or stdin prompt loop
	fmt.Print("\nEnter command (s=simulate, r=reset, +=more chunks, -=fewer chunks, q=quit): ")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
		fmt.Print("\033[2J\033[H")
		fmt.Println("Prototype exited.")
		os.Exit(0)
	}()

	for {
		var input string
		_, err := fmt.Scanln(&input)
		if err != nil {
			input = ""
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input == "q" || input == "quit" {
			fmt.Print("\033[2J\033[H")
			fmt.Println("Prototype terminated.")
			return
		}

		mu.Lock()
		currentRunning := isRunning
		mu.Unlock()

		if currentRunning {
			continue
		}

		switch input {
		case "s", "sim", "simulate":
			mu.Lock()
			isRunning = true
			statusMsg = "Simulating parallel HTTP Range chunk downloads..."
			mu.Unlock()

			simCtx, simCancel := context.WithTimeout(ctx, 30*time.Second)
			go func() {
				defer simCancel()
				job.Simulate(simCtx, concurrency, func() {
					mu.Lock()
					defer mu.Unlock()
					renderFrame(job, concurrency, true, "Simulating parallel download...")
				})

				mu.Lock()
				isRunning = false
				statusMsg = "Simulation COMPLETED successfully!"
				renderFrame(job, concurrency, false, statusMsg)
				fmt.Print("\nEnter command (s=simulate, r=reset, +=more chunks, -=fewer chunks, q=quit): ")
				mu.Unlock()
			}()

		case "r", "reset":
			job = downloader.NewDownloadJob("https://example.com/sample_video.mp4", "/tmp/grab_sample.mp4", totalSize, numChunks, nil)
			statusMsg = "Job state reset."
			renderFrame(job, concurrency, false, statusMsg)
			fmt.Print("\nEnter command (s=simulate, r=reset, +=more chunks, -=fewer chunks, q=quit): ")

		case "+":
			if numChunks < 32 {
				numChunks++
				job = downloader.NewDownloadJob("https://example.com/sample_video.mp4", "/tmp/grab_sample.mp4", totalSize, numChunks, nil)
				statusMsg = fmt.Sprintf("Increased chunk count to %d.", numChunks)
			}
			renderFrame(job, concurrency, false, statusMsg)
			fmt.Print("\nEnter command (s=simulate, r=reset, +=more chunks, -=fewer chunks, q=quit): ")

		case "-":
			if numChunks > 1 {
				numChunks--
				job = downloader.NewDownloadJob("https://example.com/sample_video.mp4", "/tmp/grab_sample.mp4", totalSize, numChunks, nil)
				statusMsg = fmt.Sprintf("Decreased chunk count to %d.", numChunks)
			}
			renderFrame(job, concurrency, false, statusMsg)
			fmt.Print("\nEnter command (s=simulate, r=reset, +=more chunks, -=fewer chunks, q=quit): ")

		default:
			renderFrame(job, concurrency, false, statusMsg)
			fmt.Print("\nEnter command (s=simulate, r=reset, +=more chunks, -=fewer chunks, q=quit): ")
		}
	}
}
