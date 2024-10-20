package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime/trace"
	"strconv"
	"sync"
	"time"

	_ "github.com/lib/pq" // Import the PostgreSQL driver
)

// Store active requests and their cancel functions
var activeRequests = make(map[string]context.CancelFunc)
var mu sync.Mutex // Mutex to protect the activeRequests map

// Database connection
var db *sql.DB

type Requests struct {
	id          int
	ip          string
	matrix_size int
	created_at  time.Time
	// Add more fields based on your database schema
}

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

// storeRequestInfo stores the IP and matrix size in the PostgreSQL database.
func storeRequestInfo(ip string, matrixSize int) error {
	_, err := db.Exec("INSERT INTO requests (ip, matrix_size) VALUES ($1, $2)", ip, matrixSize)
	return err
}

// migrate ensures the requests table exists in the database.
func migrate() error {
	// SQL command to create the requests table if it does not exist
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS requests (
		id SERIAL PRIMARY KEY,
		ip VARCHAR(255),
		matrix_size INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := db.Exec(createTableSQL)
	return err
}

// createDatabase creates the specified database if it doesn't exist.
func createDatabase(dbName, username, password string) error {
	// Connect to the default postgres database
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", username, password, dbName)
	tempDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to default database: %w", err)
	}
	defer tempDB.Close()

	// Create the database
	_, err = tempDB.Exec(fmt.Sprintf("CREATE DATABASE %s;", dbName))
	return err
}

// checkDatabaseExists verifies if the database exists.
func checkDatabaseExists(dbName, username, host, password string) (bool, error) {
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s sslmode=disable", username, password, dbName, host)
	tempDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return false, fmt.Errorf("failed to connect to default database: %w", err)
	}

	defer tempDB.Close()

	var exists bool
	err = tempDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", dbName).Scan(&exists)
	return exists, err
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

	if err != nil || matrixSize <= 0 {
		matrixSize = 100 // Default size if invalid or not provided
	}

	if matrixSize > 2000 {
		select {
		case <-ctx.Done():
			fmt.Fprintf(w, "Request was cancelled due to large number\n")
		}
	} else {
		// Store the request info in the database
		if err := storeRequestInfo(ip, matrixSize); err != nil {
			http.Error(w, "Failed to store request info", http.StatusInternalServerError)
			return
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

func getRequestInfo(ip string, page int, pageSize int) ([]Requests, error) {
	// Define the offset for pagination
	//offset := (page - 1) * pageSize

	// Create the SQL query
	query := `SELECT * FROM requests WHERE IP = $1 ORDER BY id `

	// Prepare a slice to hold your results
	var results []Requests

	// Execute the query
	rows, err := db.Query(query, ip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Iterate through the rows
	for rows.Next() {
		var data Requests
		if err := rows.Scan(&data.id, &data.ip, &data.matrix_size, &data.created_at); err != nil {
			return nil, err
		}
		results = append(results, data)

	}
	// Check for any error encountered during iteration
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// Serve the HTML page
func servePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func allValues(w http.ResponseWriter, r *http.Request) {
	// Parse the page and pageSize from query parameters
	page := r.URL.Query().Get("page")
	pageSize := r.URL.Query().Get("pageSize")

	// Convert string parameters to integers (you may want to handle potential errors)
	currentPage := 1
	currentPageSize := 10

	if page != "" {
		p, err := strconv.Atoi(page)
		if err == nil {
			currentPage = p
		}
	}

	if pageSize != "" {
		ps, err := strconv.Atoi(pageSize)
		if err == nil {
			currentPageSize = ps
		}
	}

	ip := extractIP(r.RemoteAddr)

	// Get request info
	requests, err := getRequestInfo(ip, currentPage, currentPageSize)
	if err != nil {
		http.Error(w, "Unable to fetch data", http.StatusInternalServerError)
		return
	}

	// Prepare the HTML snippet for the response
	var responseHtml = `<table class="table" ; color: black;">
    <thead>
        <tr>
            <th>ID</th>
            <th>IP</th>
            <th>Size</th>
            <th>Created At</th>
        </tr>
    </thead>
    <tbody>`

	for _, req := range requests {
		responseHtml += fmt.Sprintf(`
				<tr>
					<td>%d</td>
					<td>%s</td>
					<td>%d</td>
					<td>%s</td>
				</tr>`,
			req.id, req.ip, req.matrix_size, req.created_at.Format("2006-01-02 15:04:05"))
	}

	responseHtml += `</tbody></table>`

	// Optionally, add pagination controls if needed
	// responseHtml += `<div>...pagination links...</div>`

	// Write the HTML response
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(responseHtml))
}

func main() {
	// Connect to PostgreSQL database using environment variables
	var err error
	username := os.Getenv("DB_USERNAME")
	password := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	host := os.Getenv("DB_HOST")
	fmt.Println(username, password, dbName, host)
	// Check if the database exists
	exists, err := checkDatabaseExists(dbName, username, host, password)
	if err != nil {
		fmt.Println("Error checking database existence:", err)
		return
	}

	// Create the database if it doesn't exist
	if !exists {
		if err := createDatabase(dbName, username, password); err != nil {
			fmt.Println("Error creating database:", err)
			return
		}
		fmt.Printf("Database %s created successfully.\n", dbName)
	}

	// Now connect to the specific database
	connStr := fmt.Sprintf("user=%s dbname=%s password=%s host=%s sslmode=disable", username, dbName, password, host)
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		fmt.Println("Failed to connect to database:", err)
		return
	}
	fmt.Println(db)
	defer db.Close()

	// Run migration to ensure the requests table exists
	if err := migrate(); err != nil {
		fmt.Println("Migration failed:", err)
		return
	}

	f, _ := os.Create("trace.out")
	trace.Start(f)
	defer trace.Stop()

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	http.HandleFunc("/cpu-intensive", handler)
	http.HandleFunc("/all-values", allValues)
	http.HandleFunc("/", servePage)

	port := 8080
	fmt.Printf("Starting server on port %d\n", port)
	err = http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
