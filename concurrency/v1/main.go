package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime/trace"
	"strconv"
	"sync"
	"time"
)

// Store active requests and their cancel functions
var activeRequests = make(map[string]context.CancelFunc)
var mu sync.Mutex // Mutex to protect the activeRequests map

// generateMatrix creates a matrix of given size with random values.
func generateMatrix(size int) [][]int {
	matrix := make([][]int, size)
	for i := range matrix {
		matrix[i] = make([]int, size)
		for j := range matrix[i] {
			matrix[i][j] = rand.Intn(100)
		}
	}
	return matrix
}

// multiplyRow multiplies a row of matrix A by matrix B and stores the result in result matrix.
func multiplyRow(ctx context.Context, a, b, result [][]int, row, size int, wg *sync.WaitGroup) {
	defer wg.Done()
	for j := 0; j < size; j++ {
		select {
		case <-ctx.Done(): // If the request is canceled, stop processing
			return
		default:
			for k := 0; k < size; k++ {
				result[row][j] += a[row][k] * b[k][j]
			}
		}
	}
}

// multiplyMatricesParallel performs matrix multiplication using goroutines with context for cancellation.
func multiplyMatricesParallel(ctx context.Context, a, b [][]int, size int) [][]int {
	result := make([][]int, size)
	for i := range result {
		result[i] = make([]int, size)
	}

	var wg sync.WaitGroup

	for i := 0; i < size; i++ {
		wg.Add(1)
		go multiplyRow(ctx, a, b, result, i, size, &wg)
	}

	wg.Wait()
	return result
}

// cancelPreviousRequest cancels all previous requests from the same IP.
func cancelPreviousRequest(ip string) {
	mu.Lock()
	defer mu.Unlock()
	if cancelFunc, exists := activeRequests[ip]; exists {
		cancelFunc() // Cancel the previous request
		delete(activeRequests, ip)
	}
}

// extractIP gets the IP address from r.RemoteAddr (ignoring the port number).
func extractIP(remoteAddr string) string {
	if ip, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return ip
	}
	return remoteAddr // In case the address has no port
}

// handler for CPU-intensive task of matrix multiplication with cancellation support.
func handler(w http.ResponseWriter, r *http.Request) {
	// Extract client IP
	ip := extractIP(r.RemoteAddr)
	fmt.Println(ip)
	// Cancel any previous request from this IP
	cancelPreviousRequest(ip)

	// Create a new context with cancellation for this request
	ctx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	activeRequests[ip] = cancel
	mu.Unlock()

	// Get the matrix size from the URL query parameter, defaulting to 100 if not provided
	matrixSizeParam := r.URL.Query().Get("size")
	matrixSize, err := strconv.Atoi(matrixSizeParam)

	if matrixSize > 2000 {
		select {
		case <-ctx.Done():
			fmt.Fprintf(w, "Request was cancelled due to large number\n")
		}
	} else {
		if err != nil || matrixSize <= 0 {
			matrixSize = 100 // Default size if invalid or not provided
		}

		start := time.Now()

		// Generate two random matrices
		a := generateMatrix(matrixSize)
		b := generateMatrix(matrixSize)

		// Multiply the matrices using goroutines and context for cancellation
		result := multiplyMatricesParallel(ctx, a, b, matrixSize)

		duration := time.Since(start).Milliseconds()
		select {
		case <-ctx.Done():
			fmt.Fprintf(w, "Request was cancelled\n")
		default:
			// Send the matrix and the duration back to the client
			fmt.Fprintf(w, "Matrix multiplication (size: %d) completed in %d milliseconds\n", matrixSize, duration)
			fmt.Fprintf(w, "Resulting Matrix:\n")
			for _, row := range result {
				fmt.Fprintf(w, "%v\n", row)
			}
		}
	}
}

// Serve the HTML page
func servePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func main() {
	f, _ := os.Create("trace.out")
	trace.Start(f)
	defer trace.Stop()

	http.HandleFunc("/cpu-intensive", handler)
	http.HandleFunc("/", servePage)

	port := 8080
	fmt.Printf("Starting server on port %d\n", port)
	err := http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
