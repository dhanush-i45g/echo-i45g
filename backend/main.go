package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ============================================================================
// Network PC Monitoring System - Backend Server
// ============================================================================
// This service provides REST APIs for monitoring the status of computers
// on a network via ping (TCP connection on port 22).
// Uses SQLite for persistent storage.
// ============================================================================

// Computer represents a network computer in the monitoring system
type Computer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// ComputerInput is the request body for adding a new computer (no ID needed)
type ComputerInput struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// ComputerStatus contains the current status of a computer after a ping check
type ComputerStatus struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Status    string `json:"status"`    // "ON" or "OFF"
	CheckedAt string `json:"checkedAt"` // Timestamp of last check
}

// APIResponse is the standard response format for all API endpoints
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ============================================================================
// Database
// ============================================================================

// db is the global SQLite database connection
var db *sql.DB

// initDB opens the SQLite database and creates the computers table if needed
func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./monitor.db")
	if err != nil {
		log.Fatalf("FATAL: Failed to open database: %v", err)
	}

	// Enable WAL mode for better concurrent read performance
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		log.Printf("WARNING: Failed to set WAL mode: %v", err)
	}

	// Create computers table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS computers (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT    NOT NULL,
			ip   TEXT    NOT NULL UNIQUE
		)
	`)
	if err != nil {
		log.Fatalf("FATAL: Failed to create computers table: %v", err)
	}

	log.Println("INFO: Database initialized successfully")
}

// getAllComputers retrieves all computers from the database
func getAllComputers() ([]Computer, error) {
	rows, err := db.Query("SELECT id, name, ip FROM computers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	computers := []Computer{}
	for rows.Next() {
		var c Computer
		if err := rows.Scan(&c.ID, &c.Name, &c.IP); err != nil {
			return nil, err
		}
		computers = append(computers, c)
	}
	return computers, rows.Err()
}

// getComputerByID retrieves a single computer by its ID
func getComputerByID(id string) (*Computer, error) {
	var c Computer
	err := db.QueryRow("SELECT id, name, ip FROM computers WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.IP)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// insertComputer adds a new computer to the database and returns it with its generated ID
func insertComputer(name, ip string) (*Computer, error) {
	result, err := db.Exec("INSERT INTO computers (name, ip) VALUES (?, ?)", name, ip)
	if err != nil {
		return nil, err
	}
	newID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Computer{ID: int(newID), Name: name, IP: ip}, nil
}

// ============================================================================
// Network Utilities
// ============================================================================

// pingHost uses the system's ping command to check if a host is reachable via ICMP.
// Returns "ON" if the host responds, "OFF" otherwise.
func pingHost(ip string) string {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "1", "-w", "2000", ip)
	} else {
		cmd = exec.Command("ping", "-c", "1", "-W", "2", ip)
	}

	if err := cmd.Run(); err != nil {
		return "OFF"
	}
	return "ON"
}

// ============================================================================
// HTTP Utilities
// ============================================================================

// setCORS configures CORS headers for cross-origin requests
func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
}

// writeJSON writes a JSON response with the specified HTTP status code
func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("ERROR: Failed to encode JSON response: %v", err)
	}
}

// ============================================================================
// API Endpoints
// ============================================================================

// listComputers handles GET /api/computers
// Returns all computers from the database
func listComputers(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	computers, err := getAllComputers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "Failed to retrieve computers",
		})
		log.Printf("ERROR: Failed to list computers: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    computers,
	})
	log.Printf("INFO: Listed %d computers", len(computers))
}

// addComputer handles POST /api/computers
// Adds a new computer to the database with validation. ID is auto-generated.
func addComputer(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	// Parse request body
	var input ComputerInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "Invalid JSON body",
		})
		log.Printf("ERROR: Failed to decode computer data: %v", err)
		return
	}

	// Validate required fields
	if input.Name == "" || input.IP == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "name and ip are required",
		})
		log.Println("WARNING: Attempt to add computer with missing fields")
		return
	}

	// Insert into database (UNIQUE constraint on ip handles duplicates)
	computer, err := insertComputer(input.Name, input.IP)
	if err != nil {
		// Check for duplicate IP
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeJSON(w, http.StatusConflict, APIResponse{
				Success: false,
				Error:   "A computer with this IP already exists",
			})
			log.Printf("WARNING: Attempt to add duplicate IP: %s", input.IP)
			return
		}
		writeJSON(w, http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "Failed to add computer",
		})
		log.Printf("ERROR: Failed to insert computer: %v", err)
		return
	}

	writeJSON(w, http.StatusCreated, APIResponse{
		Success: true,
		Data:    computer,
	})
	log.Printf("INFO: Computer added - ID: %d, Name: %s, IP: %s", computer.ID, computer.Name, computer.IP)
}

// deleteComputer handles DELETE /api/computers/:id
// Removes a computer from the database
func deleteComputer(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	id := strings.TrimPrefix(r.URL.Path, "/api/computers/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "Missing computer ID",
		})
		return
	}

	result, err := db.Exec("DELETE FROM computers WHERE id = ?", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "Failed to delete computer",
		})
		log.Printf("ERROR: Failed to delete computer %s: %v", id, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, APIResponse{
			Success: false,
			Error:   "Computer not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    fmt.Sprintf("Computer %s deleted", id),
	})
	log.Printf("INFO: Computer deleted - ID: %s", id)
}

// pingOne handles GET /api/ping/:id
// Pings a specific computer and returns its status
func pingOne(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	// Extract computer ID from URL path
	id := strings.TrimPrefix(r.URL.Path, "/api/ping/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error:   "Missing computer ID",
		})
		return
	}

	// Find the computer in database
	c, err := getComputerByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, APIResponse{
			Success: false,
			Error:   "Computer not found",
		})
		log.Printf("WARNING: Attempt to ping non-existent computer ID: %s", id)
		return
	}

	// Ping the computer
	status := pingHost(c.IP)
	result := ComputerStatus{
		ID:        c.ID,
		Name:      c.Name,
		IP:        c.IP,
		Status:    status,
		CheckedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    result,
	})
	log.Printf("INFO: Pinged computer %s (%s) - Status: %s", c.Name, c.IP, status)
}

// pingAll handles GET /api/ping-all
// Pings all computers concurrently and returns their statuses
func pingAll(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	computers, err := getAllComputers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{
			Success: false,
			Error:   "Failed to retrieve computers",
		})
		log.Printf("ERROR: Failed to get computers for ping-all: %v", err)
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	results := make([]ComputerStatus, len(computers))

	// Ping all computers concurrently using goroutines
	var wg sync.WaitGroup
	for i, c := range computers {
		wg.Add(1)
		go func(idx int, comp Computer) {
			defer wg.Done()
			results[idx] = ComputerStatus{
				ID:        comp.ID,
				Name:      comp.Name,
				IP:        comp.IP,
				Status:    pingHost(comp.IP),
				CheckedAt: now,
			}
		}(i, c)
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    results,
	})

	// Log summary
	onCount := 0
	for _, r := range results {
		if r.Status == "ON" {
			onCount++
		}
	}
	log.Printf("INFO: Pinged all %d computers - Online: %d, Offline: %d", len(computers), onCount, len(computers)-onCount)
}

// ============================================================================
// Router
// ============================================================================

// router is the main HTTP request handler that routes requests to appropriate endpoints
func router(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight requests
	if r.Method == http.MethodOptions {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := r.URL.Path

	// Route to appropriate handler
	switch {
	case path == "/api/computers" && r.Method == http.MethodGet:
		listComputers(w, r)
	case path == "/api/computers" && r.Method == http.MethodPost:
		addComputer(w, r)
	case strings.HasPrefix(path, "/api/computers/") && r.Method == http.MethodDelete:
		deleteComputer(w, r)
	case strings.HasPrefix(path, "/api/ping/") && r.Method == http.MethodGet:
		pingOne(w, r)
	case path == "/api/ping-all" && r.Method == http.MethodGet:
		pingAll(w, r)
	default:
		// Serve frontend static files
		http.FileServer(http.Dir("../frontend")).ServeHTTP(w, r)
	}
}

// ============================================================================
// Main
// ============================================================================

func main() {
	// Initialize database
	initDB()
	defer db.Close()

	// Register router
	http.HandleFunc("/", router)

	// Server configuration
	port := ":8081"
	log.Printf("========================================")
	log.Printf("Network PC Monitoring System - Started")
	log.Printf("Server running at http://localhost%s", port)
	log.Printf("========================================")

	// Start server
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}
