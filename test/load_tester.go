package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Simple Load Tester
func main() {
	url := "http://localhost:8080"
	concurrency := 50
	requests := 1000

	fmt.Printf("Starting Load Test: %d requests with %d concurrency...\n", requests, concurrency)

	start := time.Now()
	var wg sync.WaitGroup
	jobs := make(chan int, requests)

	// Workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				makeRequest(url)
			}
		}()
	}

	// Produce jobs
	for i := 0; i < requests; i++ {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("\nDone!\n")
	fmt.Printf("Time taken: %v\n", duration)
	fmt.Printf("Requests/sec: %.2f\n", float64(requests)/duration.Seconds())
}

func makeRequest(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
}
