package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	host       = flag.String("host", "localhost", "Redis host")
	port       = flag.Int("port", 6379, "Redis port")
	goroutines = flag.Int("goroutines", 4, "Number of concurrent goroutines")
	operations = flag.Int("operations", 10000, "Number of operations per goroutine")
	password   = flag.String("password", "", "Redis password (optional)")
)

type OperationResult struct {
	successfulOps int64
	totalLatency  time.Duration
}

func main() {
	flag.Parse()

	ctx := context.Background()

	redisAddr := *host + ":" + strconv.Itoa(*port)
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: *password,       // no password set
		DB:       0,               // use default DB
		PoolSize: *goroutines + 5, // Connection pool size
	})

	// Test connection
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	fmt.Println("Successfully connected to Redis at", redisAddr)

	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  Host: %s\n", *host)
	fmt.Printf("  Port: %d\n", *port)
	fmt.Printf("  Goroutines: %d\n", *goroutines)
	fmt.Printf("  Operations per goroutine: %d\n", *operations)
	fmt.Printf("  Total operations: %d\n", int64(*goroutines)*int64(*operations))
	if *password != "" {
		fmt.Println("  Password provided.")
	}
	fmt.Println()

	var wg sync.WaitGroup
	resultsChan := make(chan OperationResult, *goroutines)

	overallStartTime := time.Now()

	for i := 0; i < *goroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			var localSuccessfulOps int64
			var localTotalLatency time.Duration

			// Use a separate Redis client for each goroutine to avoid contention on a single client
			// This is generally recommended when each goroutine performs many operations.
			// However, for this benchmark, we will use the shared rdb client from the pool.
			// If connection pooling is efficient, this should be fine.
			// For extreme load, creating a client per goroutine might be considered,
			// but ensure proper closing or pooling for those too.

			for j := 0; j < *operations; j++ {
				key := fmt.Sprintf("goroutine-%d-op-%d", routineID, j)
				value := fmt.Sprintf("value-%d", j)

				opStartTime := time.Now()
				err := rdb.Set(ctx, key, value, 0).Err()
				if err != nil {
					// log.Printf("SET error for key %s in goroutine %d: %v", key, routineID, err)
					continue
				}

				retrievedValue, err := rdb.Get(ctx, key).Result()
				if err != nil {
					// log.Printf("GET error for key %s in goroutine %d: %v", key, routineID, err)
					continue
				}
				opEndTime := time.Now()

				if retrievedValue == value {
					localSuccessfulOps++
					localTotalLatency += opEndTime.Sub(opStartTime)
				} else {
					// log.Printf("Data mismatch for key %s in goroutine %d", key, routineID)
				}
			}
			resultsChan <- OperationResult{successfulOps: localSuccessfulOps, totalLatency: localTotalLatency}
		}(i)
	}

	wg.Wait()
	close(resultsChan)

	overallEndTime := time.Now()
	totalDuration := overallEndTime.Sub(overallStartTime)

	var totalSuccessfulOps int64
	var totalAggregatedLatency time.Duration

	for result := range resultsChan {
		totalSuccessfulOps += result.successfulOps
		totalAggregatedLatency += result.totalLatency
	}

	opsPerSecond := 0.0
	if totalDuration.Seconds() > 0 {
		opsPerSecond = float64(totalSuccessfulOps) / totalDuration.Seconds()
	}

	avgLatencyMillis := 0.0
	if totalSuccessfulOps > 0 {
		avgLatencyMillis = float64(totalAggregatedLatency.Nanoseconds()) / float64(totalSuccessfulOps) / 1e6 // Convert ns to ms
	}

	fmt.Println("\n--- Benchmark Results ---")
	fmt.Printf("Total operations performed: %d\n", totalSuccessfulOps)
	fmt.Printf("Total time taken: %.3f seconds\n", totalDuration.Seconds())
	fmt.Printf("Operations Per Second (OPS): %.2f\n", opsPerSecond)
	fmt.Printf("Average latency per operation: %.4f ms\n", avgLatencyMillis)
	fmt.Printf("Number of goroutines: %d\n", *goroutines)
	fmt.Println("------------------------")

	// Attempt to clean up keys (best effort, might be slow for huge number of keys)
	// Consider disabling for very large benchmarks or using SCAN for production cleanup.
	// fmt.Println("\nAttempting to clean up benchmark keys...")
	// cleanupStartTime := time.Now()
	// for i := 0; i < *goroutines; i++ {
	// 	for j := 0; j < *operations; j++ {
	// 		key := fmt.Sprintf("goroutine-%d-op-%d", i, j)
	// 		rdb.Del(ctx, key)
	// 	}
	// }
	// cleanupDuration := time.Since(cleanupStartTime)
	// fmt.Printf("Key cleanup took %.3f seconds (best effort).\n", cleanupDuration.Seconds())
}
