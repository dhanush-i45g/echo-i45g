package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// ============================================================================
// System Information via SSH + WebSocket
// ============================================================================
// Opens a persistent SSH connection to a device, runs system info commands
// every 2 seconds, and streams structured JSON over a WebSocket.
// ============================================================================

// SysInfo holds all collected system metrics
type SysInfo struct {
	Hostname    string     `json:"hostname"`
	OS          string     `json:"os"`
	CPU         string     `json:"cpu"`
	Cores       int        `json:"cores"`
	MemTotal    uint64     `json:"memTotal"`
	MemUsed     uint64     `json:"memUsed"`
	MemFree     uint64     `json:"memFree"`
	MemAvail    uint64     `json:"memAvail"`
	MemPercent  float64    `json:"memPercent"`
	SwapTotal   uint64     `json:"swapTotal"`
	SwapUsed    uint64     `json:"swapUsed"`
	SwapPercent float64    `json:"swapPercent"`
	DiskTotal   uint64     `json:"diskTotal"`
	DiskUsed    uint64     `json:"diskUsed"`
	DiskFree    uint64     `json:"diskFree"`
	DiskPercent float64    `json:"diskPercent"`
	Uptime      string     `json:"uptime"`
	LoadAvg     string     `json:"loadAvg"`
	Load1       float64    `json:"load1"`
	Load5       float64    `json:"load5"`
	Load15      float64    `json:"load15"`
	Networks    []NetIface `json:"networks"`
	Processes   int        `json:"processes"`
	Timestamp   string     `json:"timestamp"`
}

// NetIface represents a network interface with its IP
type NetIface struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Mask string `json:"mask"`
}

// WebSocket upgrader allows all origins (same-origin in production)
var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// sysInfoCmd is the compound SSH command that gathers all metrics in one shot
const sysInfoCmd = `echo '---HOSTNAME---' && hostname 2>/dev/null || echo 'unknown'
echo '---OS---' && uname -srm 2>/dev/null || echo 'unknown'
echo '---CPU---' && (grep 'model name' /proc/cpuinfo 2>/dev/null | head -1 | sed 's/.*: //' || echo 'unknown')
echo '---CORES---' && (nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo 2>/dev/null || echo '1')
echo '---MEM---' && free -b 2>/dev/null | grep -E '^(Mem|Swap):'
echo '---DISK---' && df -B1 / 2>/dev/null | tail -1
echo '---UPTIME---' && (uptime -s 2>/dev/null || echo 'unknown')
echo '---LOAD---' && cat /proc/loadavg 2>/dev/null || echo '0 0 0 0 0'
echo '---NET---' && (ip -4 -o addr show 2>/dev/null | grep -v '127.0.0.1' || ifconfig 2>/dev/null | grep 'inet ' | grep -v '127.0.0.1')
echo '---PROCS---' && (ls /proc 2>/dev/null | grep -c '^[0-9]' || echo '0')`

// connectSSH establishes an SSH connection using a private key
func connectSSH(ip string, port int, user, privateKey string) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("invalid SSH private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", ip, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to %s failed: %v", addr, err)
	}
	return client, nil
}

// collectSysInfo runs the compound command and parses the output
func collectSysInfo(client *ssh.Client) (*SysInfo, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("SSH session failed: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(sysInfoCmd)
	if err != nil && len(output) == 0 {
		return nil, fmt.Errorf("command execution failed: %v", err)
	}

	return parseSysInfo(string(output))
}

// parseSysInfo parses the delimited command output into a SysInfo struct
func parseSysInfo(output string) (*SysInfo, error) {
	info := &SysInfo{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
	}

	// Split output into sections by delimiter markers
	sections := make(map[string]string)
	currentSection := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "---") && strings.HasSuffix(line, "---") {
			currentSection = line
			continue
		}
		if currentSection != "" && line != "" {
			if existing, ok := sections[currentSection]; ok {
				sections[currentSection] = existing + "\n" + line
			} else {
				sections[currentSection] = line
			}
		}
	}

	// Hostname
	info.Hostname = strings.TrimSpace(sections["---HOSTNAME---"])

	// OS
	info.OS = strings.TrimSpace(sections["---OS---"])

	// CPU model
	info.CPU = strings.TrimSpace(sections["---CPU---"])

	// Core count
	if cores, err := strconv.Atoi(strings.TrimSpace(sections["---CORES---"])); err == nil {
		info.Cores = cores
	} else {
		info.Cores = 1
	}

	// Memory: parse free -b output
	// Mem:  total  used  free  shared  buff/cache  available
	// Swap: total  used  free
	memBlock := sections["---MEM---"]
	for _, memLine := range strings.Split(memBlock, "\n") {
		fields := strings.Fields(memLine)
		if len(fields) < 4 {
			continue
		}
		if strings.HasPrefix(fields[0], "Mem") {
			total, _ := strconv.ParseUint(fields[1], 10, 64)
			used, _ := strconv.ParseUint(fields[2], 10, 64)
			free, _ := strconv.ParseUint(fields[3], 10, 64)
			info.MemTotal = total
			info.MemUsed = used
			info.MemFree = free
			if len(fields) >= 7 {
				avail, _ := strconv.ParseUint(fields[6], 10, 64)
				info.MemAvail = avail
			}
			if total > 0 {
				info.MemPercent = float64(used) / float64(total) * 100
			}
		} else if strings.HasPrefix(fields[0], "Swap") {
			total, _ := strconv.ParseUint(fields[1], 10, 64)
			used, _ := strconv.ParseUint(fields[2], 10, 64)
			info.SwapTotal = total
			info.SwapUsed = used
			if total > 0 {
				info.SwapPercent = float64(used) / float64(total) * 100
			}
		}
	}

	// Disk: df -B1 / output
	// filesystem  1B-blocks  used  available  use%  mount
	diskLine := strings.TrimSpace(sections["---DISK---"])
	if diskLine != "" {
		fields := strings.Fields(diskLine)
		if len(fields) >= 4 {
			total, _ := strconv.ParseUint(fields[1], 10, 64)
			used, _ := strconv.ParseUint(fields[2], 10, 64)
			free, _ := strconv.ParseUint(fields[3], 10, 64)
			info.DiskTotal = total
			info.DiskUsed = used
			info.DiskFree = free
			if total > 0 {
				info.DiskPercent = float64(used) / float64(total) * 100
			}
		}
	}

	// Uptime
	info.Uptime = strings.TrimSpace(sections["---UPTIME---"])

	// Load average: /proc/loadavg format: 1min 5min 15min running/total lastpid
	loadLine := strings.TrimSpace(sections["---LOAD---"])
	if loadLine != "" {
		fields := strings.Fields(loadLine)
		if len(fields) >= 3 {
			info.Load1, _ = strconv.ParseFloat(fields[0], 64)
			info.Load5, _ = strconv.ParseFloat(fields[1], 64)
			info.Load15, _ = strconv.ParseFloat(fields[2], 64)
			info.LoadAvg = fmt.Sprintf("%.2f  %.2f  %.2f", info.Load1, info.Load5, info.Load15)
		}
	}

	// Network interfaces
	netBlock := sections["---NET---"]
	for _, line := range strings.Split(netBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ip -4 -o addr show format: "2: eth0    inet 192.168.1.100/24 ..."
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			ifaceName := strings.TrimSuffix(fields[1], ":")
			for i, f := range fields {
				if f == "inet" && i+1 < len(fields) {
					parts := strings.SplitN(fields[i+1], "/", 2)
					ip := parts[0]
					mask := ""
					if len(parts) > 1 {
						mask = "/" + parts[1]
					}
					info.Networks = append(info.Networks, NetIface{
						Name: ifaceName,
						IP:   ip,
						Mask: mask,
					})
					break
				}
			}
		}
	}

	// Process count
	procsLine := strings.TrimSpace(sections["---PROCS---"])
	if procsLine != "" {
		info.Processes, _ = strconv.Atoi(procsLine)
	}

	return info, nil
}

// handleSysInfoWS is the WebSocket handler for live system info streaming
func handleSysInfoWS(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ws/sysinfo/")
	if id == "" {
		http.Error(w, "Missing computer ID", http.StatusBadRequest)
		return
	}

	comp, err := getComputerByID(id)
	if err != nil {
		http.Error(w, "Computer not found", http.StatusNotFound)
		return
	}

	if comp.SSHKey == "" {
		http.Error(w, "No SSH private key configured for this device", http.StatusBadRequest)
		return
	}

	sshUser := comp.SSHUser
	if sshUser == "" {
		sshUser = "root"
	}

	// Upgrade HTTP to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ERROR: WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("INFO: SysInfo WS opened for %s (%s@%s)", comp.Name, sshUser, comp.IP)

	// Establish SSH connection
	sshClient, err := connectSSH(comp.IP, 22, sshUser, comp.SSHKey)
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": err.Error()})
		log.Printf("ERROR: SSH connect failed for %s: %v", comp.Name, err)
		return
	}
	defer sshClient.Close()

	// Notify frontend of successful connection
	_ = conn.WriteJSON(map[string]interface{}{
		"connected": true,
		"host":      comp.Name,
		"ip":        comp.IP,
		"user":      sshUser,
	})

	// Goroutine to detect client disconnect
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Collect and send first batch immediately
	if info, err := collectSysInfo(sshClient); err == nil {
		_ = conn.WriteJSON(info)
	} else {
		_ = conn.WriteJSON(map[string]string{"error": err.Error()})
	}

	// Stream every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			log.Printf("INFO: SysInfo WS closed for %s", comp.Name)
			return
		case <-ticker.C:
			info, err := collectSysInfo(sshClient)
			if err != nil {
				errMsg := map[string]string{"error": fmt.Sprintf("Command failed: %v", err)}
				if writeErr := conn.WriteJSON(errMsg); writeErr != nil {
					log.Printf("INFO: SysInfo write failed for %s, closing", comp.Name)
					return
				}
				continue
			}
			if writeErr := conn.WriteJSON(info); writeErr != nil {
				log.Printf("INFO: SysInfo write failed for %s, closing", comp.Name)
				return
			}
		}
	}
}
