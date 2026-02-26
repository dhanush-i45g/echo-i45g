package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
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
// REST APIs for monitoring computers on a network via ICMP ping.
// Uses SQLite for persistent storage. Supports SSH key collection
// for future SSH-based communication.
// ============================================================================

// Computer represents a network computer in the monitoring system
type Computer struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	SSHKey    string `json:"sshKey"`
	SSHUser   string `json:"sshUser"`
	Status    string `json:"status"`
	CheckedAt string `json:"checkedAt"`
}

// ComputerInput is the request body for adding/updating a computer
type ComputerInput struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	SSHKey  string `json:"sshKey"`
	SSHUser string `json:"sshUser"`
}

// ComputerStatus contains the current status of a computer after a ping check
type ComputerStatus struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	SSHKey    string `json:"sshKey"`
	SSHUser   string `json:"sshUser"`
	Status    string `json:"status"`    // "ON" or "OFF"
	CheckedAt string `json:"checkedAt"` // Timestamp of last check
}

// CSVUploadResult contains the result of a bulk CSV upload
type CSVUploadResult struct {
	Added   int      `json:"added"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
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

var db *sql.DB

// initDB opens the SQLite database and creates/migrates the computers table
func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./monitor.db")
	if err != nil {
		log.Fatalf("FATAL: Failed to open database: %v", err)
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL")

	// Create table with all columns
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS computers (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			ip         TEXT NOT NULL UNIQUE,
			ssh_key    TEXT NOT NULL DEFAULT '',
			ssh_user   TEXT NOT NULL DEFAULT 'root',
			status     TEXT NOT NULL DEFAULT '',
			checked_at TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		log.Fatalf("FATAL: Failed to create computers table: %v", err)
	}

	// Migrations for existing databases
	_, _ = db.Exec("ALTER TABLE computers ADD COLUMN ssh_key TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE computers ADD COLUMN ssh_user TEXT NOT NULL DEFAULT 'root'")
	_, _ = db.Exec("ALTER TABLE computers ADD COLUMN status TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE computers ADD COLUMN checked_at TEXT NOT NULL DEFAULT ''")

	log.Println("INFO: Database initialized successfully")
}

// getAllComputers retrieves all computers from the database
func getAllComputers() ([]Computer, error) {
	rows, err := db.Query("SELECT id, name, ip, ssh_key, ssh_user, status, checked_at FROM computers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	computers := []Computer{}
	for rows.Next() {
		var c Computer
		if err := rows.Scan(&c.ID, &c.Name, &c.IP, &c.SSHKey, &c.SSHUser, &c.Status, &c.CheckedAt); err != nil {
			return nil, err
		}
		computers = append(computers, c)
	}
	return computers, rows.Err()
}

// getComputerByID retrieves a single computer by its ID
func getComputerByID(id string) (*Computer, error) {
	var c Computer
	err := db.QueryRow("SELECT id, name, ip, ssh_key, ssh_user, status, checked_at FROM computers WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.IP, &c.SSHKey, &c.SSHUser, &c.Status, &c.CheckedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// insertComputer adds a new computer and returns it with its generated ID
func insertComputer(name, ip, sshKey, sshUser string) (*Computer, error) {
	if sshUser == "" {
		sshUser = "root"
	}
	result, err := db.Exec("INSERT INTO computers (name, ip, ssh_key, ssh_user) VALUES (?, ?, ?, ?)", name, ip, sshKey, sshUser)
	if err != nil {
		return nil, err
	}
	newID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Computer{ID: int(newID), Name: name, IP: ip, SSHKey: sshKey, SSHUser: sshUser}, nil
}

// updateComputer updates an existing computer's fields
func updateComputerDB(id string, name, ip, sshKey, sshUser string) (*Computer, error) {
	if sshUser == "" {
		sshUser = "root"
	}
	result, err := db.Exec("UPDATE computers SET name = ?, ip = ?, ssh_key = ?, ssh_user = ? WHERE id = ?", name, ip, sshKey, sshUser, id)
	if err != nil {
		return nil, err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, fmt.Errorf("computer not found")
	}
	return getComputerByID(id)
}

// ============================================================================
// Network Utilities
// ============================================================================

// pingHost uses the system's ping command to check if a host is reachable via ICMP.
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

// saveStatus persists the ping result into the database
func saveStatus(id int, status, checkedAt string) {
	_, err := db.Exec("UPDATE computers SET status = ?, checked_at = ? WHERE id = ?", status, checkedAt, id)
	if err != nil {
		log.Printf("ERROR: Failed to save status for computer %d: %v", id, err)
	}
}

// ============================================================================
// HTTP Utilities
// ============================================================================

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
}

func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("ERROR: Failed to encode JSON response: %v", err)
	}
}

// ============================================================================
// API Endpoints
// ============================================================================

// GET /api/computers
func listComputers(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	computers, err := getAllComputers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: "Failed to retrieve computers"})
		log.Printf("ERROR: Failed to list computers: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: computers})
	log.Printf("INFO: Listed %d computers", len(computers))
}

// POST /api/computers
func addComputer(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	var input ComputerInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "Invalid JSON body"})
		return
	}

	if input.Name == "" || input.IP == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "name and ip are required"})
		return
	}

	computer, err := insertComputer(input.Name, input.IP, input.SSHKey, input.SSHUser)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeJSON(w, http.StatusConflict, APIResponse{Success: false, Error: "A computer with this IP already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: "Failed to add computer"})
		log.Printf("ERROR: Failed to insert computer: %v", err)
		return
	}

	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: computer})
	log.Printf("INFO: Computer added - ID: %d, Name: %s, IP: %s", computer.ID, computer.Name, computer.IP)
}

// PUT /api/computers/:id
func updateComputer(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	id := strings.TrimPrefix(r.URL.Path, "/api/computers/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "Missing computer ID"})
		return
	}

	var input ComputerInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "Invalid JSON body"})
		return
	}

	if input.Name == "" || input.IP == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "name and ip are required"})
		return
	}

	computer, err := updateComputerDB(id, input.Name, input.IP, input.SSHKey, input.SSHUser)
	if err != nil {
		if err.Error() == "computer not found" {
			writeJSON(w, http.StatusNotFound, APIResponse{Success: false, Error: "Computer not found"})
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeJSON(w, http.StatusConflict, APIResponse{Success: false, Error: "A computer with this IP already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: "Failed to update computer"})
		log.Printf("ERROR: Failed to update computer %s: %v", id, err)
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: computer})
	log.Printf("INFO: Computer updated - ID: %s, Name: %s, IP: %s", id, computer.Name, computer.IP)
}

// DELETE /api/computers/:id
func deleteComputer(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	id := strings.TrimPrefix(r.URL.Path, "/api/computers/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "Missing computer ID"})
		return
	}

	result, err := db.Exec("DELETE FROM computers WHERE id = ?", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: "Failed to delete computer"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, APIResponse{Success: false, Error: "Computer not found"})
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: fmt.Sprintf("Computer %s deleted", id)})
	log.Printf("INFO: Computer deleted - ID: %s", id)
}

// POST /api/computers/upload - CSV bulk upload
// Expected CSV format: name,ip,ssh_key (header row optional)
func uploadCSV(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	// Limit upload size to 5MB
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "No file uploaded or file too large"})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	var added, skipped int
	var errors []string
	lineNum := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		lineNum++
		if err != nil {
			errors = append(errors, fmt.Sprintf("Line %d: invalid CSV format", lineNum))
			continue
		}

		// Skip header row if detected
		if lineNum == 1 && len(record) > 0 {
			lower := strings.ToLower(strings.TrimSpace(record[0]))
			if lower == "name" || lower == "hostname" || lower == "computer" {
				continue
			}
		}

		if len(record) < 2 {
			errors = append(errors, fmt.Sprintf("Line %d: need at least name and ip columns", lineNum))
			skipped++
			continue
		}

		name := strings.TrimSpace(record[0])
		ip := strings.TrimSpace(record[1])
		sshKey := ""
		if len(record) >= 3 {
			sshKey = strings.TrimSpace(record[2])
		}
		sshUser := "root"
		if len(record) >= 4 {
			sshUser = strings.TrimSpace(record[3])
		}

		if name == "" || ip == "" {
			errors = append(errors, fmt.Sprintf("Line %d: name and ip cannot be empty", lineNum))
			skipped++
			continue
		}

		_, insertErr := insertComputer(name, ip, sshKey, sshUser)
		if insertErr != nil {
			if strings.Contains(insertErr.Error(), "UNIQUE constraint failed") {
				errors = append(errors, fmt.Sprintf("Line %d: IP %s already exists (skipped)", lineNum, ip))
			} else {
				errors = append(errors, fmt.Sprintf("Line %d: %v", lineNum, insertErr))
			}
			skipped++
			continue
		}
		added++
	}

	result := CSVUploadResult{Added: added, Skipped: skipped, Errors: errors}
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: result})
	log.Printf("INFO: CSV upload - Added: %d, Skipped: %d", added, skipped)
}

// GET /api/ping/:id
func pingOne(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	id := strings.TrimPrefix(r.URL.Path, "/api/ping/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: "Missing computer ID"})
		return
	}

	c, err := getComputerByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, APIResponse{Success: false, Error: "Computer not found"})
		return
	}

	status := pingHost(c.IP)
	checkedAt := time.Now().Format("2006-01-02 15:04:05")
	saveStatus(c.ID, status, checkedAt)
	result := ComputerStatus{
		ID: c.ID, Name: c.Name, IP: c.IP, SSHKey: c.SSHKey, SSHUser: c.SSHUser,
		Status: status, CheckedAt: checkedAt,
	}
	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: result})
	log.Printf("INFO: Pinged %s (%s) - %s", c.Name, c.IP, status)
}

// GET /api/ping-all
func pingAll(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	computers, err := getAllComputers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: "Failed to retrieve computers"})
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	results := make([]ComputerStatus, len(computers))

	var wg sync.WaitGroup
	for i, c := range computers {
		wg.Add(1)
		go func(idx int, comp Computer) {
			defer wg.Done()
			s := pingHost(comp.IP)
			results[idx] = ComputerStatus{
				ID: comp.ID, Name: comp.Name, IP: comp.IP, SSHKey: comp.SSHKey, SSHUser: comp.SSHUser,
				Status: s, CheckedAt: now,
			}
			saveStatus(comp.ID, s, now)
		}(i, c)
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: results})

	onCount := 0
	for _, r := range results {
		if r.Status == "ON" {
			onCount++
		}
	}
	log.Printf("INFO: Pinged all %d - Online: %d, Offline: %d", len(computers), onCount, len(computers)-onCount)
}

// ============================================================================
// Router
// ============================================================================

func router(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := r.URL.Path

	switch {
	case strings.HasPrefix(path, "/ws/sysinfo/"):
		handleSysInfoWS(w, r)
	case path == "/api/computers" && r.Method == http.MethodGet:
		listComputers(w, r)
	case path == "/api/computers" && r.Method == http.MethodPost:
		addComputer(w, r)
	case path == "/api/computers/upload" && r.Method == http.MethodPost:
		uploadCSV(w, r)
	case strings.HasPrefix(path, "/api/computers/") && r.Method == http.MethodPut:
		updateComputer(w, r)
	case strings.HasPrefix(path, "/api/computers/") && r.Method == http.MethodDelete:
		deleteComputer(w, r)
	case strings.HasPrefix(path, "/api/ping/") && r.Method == http.MethodGet:
		pingOne(w, r)
	case path == "/api/ping-all" && r.Method == http.MethodGet:
		pingAll(w, r)
	default:
		http.FileServer(http.Dir("../frontend")).ServeHTTP(w, r)
	}
}

// ============================================================================
// Main
// ============================================================================

func main() {
	initDB()
	defer db.Close()

	http.HandleFunc("/", router)

	port := ":8081"
	log.Printf("========================================")
	log.Printf("Echo i45G - Network Monitor Started")
	log.Printf("Server running at http://localhost%s", port)
	log.Printf("========================================")

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}
