package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/montanaflynn/stats" // For percentile calculations
)

// OperationResult stores the outcome of a single command execution.
type OperationResult struct {
	Latency time.Duration
	Error   error
}

// BenchmarkStats holds all calculated statistics.
// (This struct is defined but we are populating its fields directly in main for now)
type BenchmarkStats struct {
	TotalOps      int64
	SuccessfulOps int64 // Placeholder for now, need to count non-error results
	FailedOps     int64 // Placeholder for now
	TotalDuration time.Duration
	OpsPerSecond  float64
	MinLatency    time.Duration
	MaxLatency    time.Duration
	MeanLatency   time.Duration
	P50Latency    time.Duration
	P95Latency    time.Duration
	P99Latency    time.Duration
	P999Latency   time.Duration
	RawLatencies  []time.Duration // Already collecting this
}

var (
	targetCmdStr   *string
	rps            *int
	duration       *time.Duration
	totalOps       *int64
	parallel       *int
	reportInterval *time.Duration
)

func parseFlags() {
	targetCmdStr = flag.String("cmd", "", "The command to execute (e.g., \"redis-cli SET key value\")")
	rps = flag.Int("rps", 0, "Target operations per second (0 for unlimited)")
	duration = flag.Duration("duration", 0, "Total time to run the benchmark (e.g., \"10s\", \"1m\")")
	totalOps = flag.Int64("totalOps", 0, "Total number of operations to perform")
	parallel = flag.Int("parallel", 1, "Number of parallel workers/goroutines")
	reportInterval = flag.Duration("reportInterval", 5*time.Second, "How often to report metrics during the run (e.g., \"5s\", 0 to disable)")

	flag.Parse()

	if *targetCmdStr == "" {
		log.Fatalln("Error: -cmd flag is required.")
		flag.Usage()
	}
	if (*duration == 0 && *totalOps == 0) || (*duration != 0 && *totalOps != 0) {
		log.Fatalln("Error: Exactly one of -duration or -totalOps must be specified.")
		flag.Usage()
	}
	if *parallel <= 0 {
		log.Fatalln("Error: -parallel flag must be greater than 0.")
		flag.Usage()
	}
	if *reportInterval < 0 {
		log.Fatalln("Error: -reportInterval cannot be negative.")
		flag.Usage()
	}
}

func worker(ctx context.Context, wg *sync.WaitGroup, workerID int, cmdParts []string, resultsChan chan<- OperationResult, workerRPSLimit int) {
	defer wg.Done()
	var ticker *time.Ticker
	if workerRPSLimit > 0 {
		tickDuration := time.Second / time.Duration(workerRPSLimit)
		if tickDuration == 0 {
			tickDuration = time.Nanosecond
		}
		ticker = time.NewTicker(tickDuration)
		defer ticker.Stop()
	}

	for {
		if workerRPSLimit > 0 {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		startTime := time.Now()
		var cmd *exec.Cmd
		if len(cmdParts) > 1 {
			cmd = exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
		} else {
			cmd = exec.CommandContext(ctx, cmdParts[0])
		}
		_, err := cmd.Output()
		latency := time.Since(startTime)
		opResult := OperationResult{Latency: latency, Error: err}
		select {
		case resultsChan <- opResult:
		case <-ctx.Done():
			return
		}
	}
}

func periodicReporter(ctx context.Context, benchmarkStartTime time.Time, currentTotalOps *int64) {
	if *reportInterval == 0 {
		return
	}
	ticker := time.NewTicker(*reportInterval)
	defer ticker.Stop()
	lastReportTime := benchmarkStartTime
	var lastReportOps int64 = 0
	fmt.Println("\n--- Periodic Reports ---")
	for {
		select {
		case <-ticker.C:
			currentTime := time.Now()
			currentOps := atomic.LoadInt64(currentTotalOps)
			intervalDuration := currentTime.Sub(lastReportTime)
			intervalOps := currentOps - lastReportOps
			var intervalOpsPerSecond float64
			if intervalDuration.Seconds() > 0 {
				intervalOpsPerSecond = float64(intervalOps) / intervalDuration.Seconds()
			}
			overallDuration := currentTime.Sub(benchmarkStartTime)
			var overallOpsPerSecond float64
			if overallDuration.Seconds() > 0 {
				overallOpsPerSecond = float64(currentOps) / overallDuration.Seconds()
			}
			fmt.Printf("[%s] Current: %.2f ops/s | Total Ops: %d | Overall Avg: %.2f ops/s\n",
				time.Now().Format("15:04:05"), intervalOpsPerSecond, currentOps, overallOpsPerSecond)
			lastReportTime = currentTime
			lastReportOps = currentOps
		case <-ctx.Done():
			// fmt.Println("Periodic reporter stopping.") // Can be noisy
			return
		}
	}
}

// calculateLatencyStats calculates various latency metrics from a slice of durations.
func calculateLatencyStats(latenciesNanos []float64) (
	min time.Duration, max time.Duration, mean time.Duration,
	p50 time.Duration, p95 time.Duration, p99 time.Duration, p999 time.Duration, err error) {

	if len(latenciesNanos) == 0 {
		err = fmt.Errorf("no latencies to calculate statistics from")
		return
	}

	// Min, Max, Mean can be calculated directly or using the stats package
	minVal, _ := stats.Min(latenciesNanos)
	maxVal, _ := stats.Max(latenciesNanos)
	meanVal, _ := stats.Mean(latenciesNanos)

	min = time.Duration(minVal)
	max = time.Duration(maxVal)
	mean = time.Duration(meanVal)

	p50Val, err := stats.Percentile(latenciesNanos, 50)
	if err != nil {
		return min, max, mean, 0, 0, 0, 0, fmt.Errorf("failed to calculate p50: %w", err)
	}
	p95Val, err := stats.Percentile(latenciesNanos, 95)
	if err != nil {
		return min, max, mean, 0, 0, 0, 0, fmt.Errorf("failed to calculate p95: %w", err)
	}
	p99Val, err := stats.Percentile(latenciesNanos, 99)
	if err != nil {
		return min, max, mean, 0, 0, 0, 0, fmt.Errorf("failed to calculate p99: %w", err)
	}
	p999Val, err := stats.Percentile(latenciesNanos, 99.9)
	if err != nil {
		return min, max, mean, 0, 0, 0, 0, fmt.Errorf("failed to calculate p99.9: %w", err)
	}

	p50 = time.Duration(p50Val)
	p95 = time.Duration(p95Val)
	p99 = time.Duration(p99Val)
	p999 = time.Duration(p999Val)

	return
}

func main() {
	parseFlags()

	fmt.Println("--- Configuration ---")
	fmt.Printf("  Command: %s\n", *targetCmdStr)
	fmt.Printf("  Target RPS: %d (0 means unlimited)\n", *rps)
	if *duration > 0 {
		fmt.Printf("  Duration: %s\n", duration.String())
	} else {
		fmt.Printf("  Total Operations: %d\n", *totalOps)
	}
	fmt.Printf("  Parallel Workers: %d\n", *parallel)
	fmt.Printf("  Report Interval: %s (0 to disable)\n", reportInterval.String())

	cmdParts := strings.Fields(*targetCmdStr)
	if len(cmdParts) == 0 {
		log.Fatalln("Error: Command string is empty after parsing.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultsChan := make(chan OperationResult, *parallel*10) // Buffer size can be tuned
	var wg sync.WaitGroup
	var collectedOpsAtomic int64
	var successfulOpsAtomic int64            // For successful ops count
	var failedOpsAtomic int64                // For failed ops count
	allLatencies := make([]time.Duration, 0) // Initialize to avoid nil if no ops

	workerRPSLimit := 0
	if *rps > 0 && *parallel > 0 {
		workerRPSLimit = (*rps + *parallel - 1) / *parallel
		fmt.Printf("  RPS per worker: ~%d\n", workerRPSLimit)
	}
	fmt.Println("---------------------\nStarting benchmark...")

	benchmarkStartTime := time.Now()

	if *reportInterval > 0 {
		go periodicReporter(ctx, benchmarkStartTime, &collectedOpsAtomic)
	}

	for i := 0; i < *parallel; i++ {
		wg.Add(1)
		go worker(ctx, &wg, i, cmdParts, resultsChan, workerRPSLimit)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// var timedOut bool = false // No longer needed
	if *duration > 0 {
		go func() {
			select {
			case <-time.After(*duration):
				// timedOut = true // No longer needed
				if ctx.Err() == nil { // Check if context not already cancelled
					fmt.Println("\nBenchmark duration reached. Signaling workers to stop...")
					cancel()
				}
			case <-ctx.Done():
				return
			}
		}()
	}

COLLECT_LOOP:
	for {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				break COLLECT_LOOP
			}
			atomic.AddInt64(&collectedOpsAtomic, 1)
			allLatencies = append(allLatencies, result.Latency) // Collect all latencies
			if result.Error != nil {
				atomic.AddInt64(&failedOpsAtomic, 1)
				// log.Printf("Command error: %v", result.Error) // Can be very noisy
			} else {
				atomic.AddInt64(&successfulOpsAtomic, 1)
			}

			currentCollectedOps := atomic.LoadInt64(&collectedOpsAtomic)
			if *totalOps > 0 && currentCollectedOps >= *totalOps {
				// if !timedOut { // No longer needed with direct ctx check
				if ctx.Err() == nil { // Check if context not already cancelled
					fmt.Println("\nTarget number of operations reached. Signaling workers to stop...")
					cancel()
				}
				// }
			}
		case <-ctx.Done():
			// Context cancelled, continue draining channel
		}
	}

	// Short sleep to allow final messages from reporter or workers to print if context was just cancelled.
	time.Sleep(150 * time.Millisecond)

	actualDuration := time.Since(benchmarkStartTime)
	finalTotalOps := atomic.LoadInt64(&collectedOpsAtomic)
	finalSuccessfulOps := atomic.LoadInt64(&successfulOpsAtomic)
	finalFailedOps := atomic.LoadInt64(&failedOpsAtomic)

	fmt.Println("\n--- Final Summary ---")
	fmt.Printf("Total operations attempted: %d\n", finalTotalOps)
	fmt.Printf("Successful operations: %d\n", finalSuccessfulOps)
	fmt.Printf("Failed operations: %d\n", finalFailedOps)
	fmt.Printf("Total time taken: %s\n", actualDuration)

	if actualDuration.Seconds() > 0 {
		fmt.Printf("Overall Ops/Second (successful): %.2f\n", float64(finalSuccessfulOps)/actualDuration.Seconds())
	}

	// Calculate and print latency statistics
	if len(allLatencies) > 0 {
		latenciesNanos := make([]float64, len(allLatencies))
		for i, l := range allLatencies {
			latenciesNanos[i] = float64(l.Nanoseconds())
		}

		minLat, maxLat, meanLat, p50Lat, p95Lat, p99Lat, p999Lat, err := calculateLatencyStats(latenciesNanos)
		if err != nil {
			log.Printf("Error calculating latency stats: %v", err)
		} else {
			fmt.Println("\n--- Latency Statistics ---")
			fmt.Printf("  Min: %s\n", minLat.String())
			fmt.Printf("  Max: %s\n", maxLat.String())
			fmt.Printf("  Mean: %s\n", meanLat.String())
			fmt.Printf("  P50 (Median): %s\n", p50Lat.String())
			fmt.Printf("  P95: %s\n", p95Lat.String())
			fmt.Printf("  P99: %s\n", p99Lat.String())
			fmt.Printf("  P99.9: %s\n", p999Lat.String())
		}
	} else {
		fmt.Println("\nNo latency data collected.")
	}

	fmt.Println("---------------------------")
	fmt.Println("Benchmark finished.")
}
