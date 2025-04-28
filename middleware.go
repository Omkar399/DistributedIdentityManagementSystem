package main

import (
	"bytes"
	"context"
	"encoding/json" // Added for JSON handling
	"fmt"
	"io/ioutil" // Added for reading response bodies
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strconv" // Added for converting int to string
	"strings"
	"sync"
	"time"

	"github.com/rs/cors" // Assuming you added this earlier for CORS
)

const (
	middlewarePort = 8090
	nodeBasePort   = 8080 // From user's code
	max_Nodes      = 4    // From user's code
	pollInterval   = 5 * time.Second
	queueCapacity  = 1000
)

const (
	operationRateLimit = 1 * time.Second // 1 operation per second
)

// Request holds the details for a queued request.
type Request struct {
	w    http.ResponseWriter
	r    *http.Request
	done chan error // Channel to signal completion (should be buffered)
}

// OperationRecord holds information about operations for tracking
type OperationRecord struct {
	Type       string    `json:"type"`       // "start", "stop", "reset", "insert", "update", "delete"
	NodeID     int       `json:"nodeId"`     // Node ID for node operations
	Timestamp  time.Time `json:"timestamp"`  // When the operation was received
	Status     string    `json:"status"`     // "pending", "processing", "completed", "failed"
	Message    string    `json:"message"`    // Error message if any
	Table      string    `json:"table"`      // Table name for database operations
	Email      string    `json:"email"`      // Email for user operations (optional)
	QueryType  string    `json:"queryType"`  // Type of query for database operations
	Forwarded  bool      `json:"forwarded"`  // Whether this was forwarded to a node
	TargetNode int       `json:"targetNode"` // Target node for forwarded operations
}

// Track operations queue and provide thread safety
var (
	operationsQueue      = make([]OperationRecord, 0, 50)
	operationsQueueMutex sync.RWMutex
)

// In startOperationProcessor function around line 75
func (m *Middleware) startOperationProcessor() {
	ticker := time.NewTicker(operationRateLimit)
	defer ticker.Stop()

	for {
		<-ticker.C // Wait for ticker (1 second)

		// Process one operation from queue
		m.mutex.RLock()
		isLeaderUp := m.isLeaderUp
		leader := m.currentLeader
		m.mutex.RUnlock()

		if !isLeaderUp || leader <= 0 {
			// Skip this tick if leader is down
			continue
		}

		// ADD THIS SECTION: Verify the leader is actually in the membership list
		membershipHost := os.Getenv("MEMBERSHIP_HOST")
		if membershipHost != "" {
			members, err := getMembershipList(membershipHost)
			if err == nil {
				leaderAddr := fmt.Sprintf("node-%d:%d", leader, nodeBasePort)
				leaderActive := false

				for _, member := range members {
					if member.Address == leaderAddr {
						leaderActive = true
						break
					}
				}

				if !leaderActive {
					// Leader is not in active membership - skip this tick
					log.Printf("Leader Node %d not in active membership list - skipping operation", leader)
					continue
				}
			}
		}

		// Get next request from queue if available
		select {
		case req := <-m.requestQueue:
			// Process this request now
			targetURL, _ := url.Parse(fmt.Sprintf("http://node-%d:%d", leader, nodeBasePort))
			proxy := httputil.NewSingleHostReverseProxy(targetURL)

			// Use timeout context
			ctx, cancel := context.WithTimeout(req.r.Context(), 15*time.Second)
			defer cancel()

			// Forward the request
			err := m.forwardRequest(proxy, req.w, req.r.WithContext(ctx))
			select {
			case req.done <- err:
			default:
				log.Printf("Done channel receiver gone for rate-limited request")
			}
		default:
			// No operations in queue, do nothing this tick
		}
	}
}

// addOperation adds an operation to the queue
func addOperation(op OperationRecord) {
	operationsQueueMutex.Lock()
	defer operationsQueueMutex.Unlock()

	// Prepend the new operation (newest first)
	operationsQueue = append([]OperationRecord{op}, operationsQueue...)

	// Limit size to 50 most recent operations
	if len(operationsQueue) > 50 {
		operationsQueue = operationsQueue[:50]
	}
}

// updateOperationStatus updates the status and message of an operation
func updateOperationStatus(timestamp time.Time, status string, message string) {
	operationsQueueMutex.Lock()
	defer operationsQueueMutex.Unlock()

	for i, op := range operationsQueue {
		if op.Timestamp.Equal(timestamp) {
			operationsQueue[i].Status = status
			if message != "" {
				operationsQueue[i].Message = message
			}
			break
		}
	}
}

// getOperationsQueue returns a copy of the operations queue
func getOperationsQueue() []OperationRecord {
	operationsQueueMutex.RLock()
	defer operationsQueueMutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]OperationRecord, len(operationsQueue))
	copy(result, operationsQueue)
	return result
}

// Add this function definition to middleware.go
func getMembershipList(membershipHost string) (map[string]*MemberInfo1, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/members", membershipHost))
	if err != nil {
		return nil, fmt.Errorf("failed to get members: %v", err)
	}
	defer resp.Body.Close()

	var members map[string]*MemberInfo1
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, fmt.Errorf("failed to decode members: %v", err)
	}
	return members, nil
}

// Also add the MemberInfo1 struct definition
type MemberInfo1 struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LeaseID   int64     `json:"lease_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IsLeader  bool      `json:"is_leader"`
}

// Middleware holds the state for the middleware service.
type Middleware struct {
	currentLeader int
	mutex         sync.RWMutex
	requestQueue  chan Request
	isLeaderUp    bool
	validRegions  map[string]bool
}

// NewMiddleware creates and initializes a new Middleware instance.
func NewMiddleware() *Middleware {
	m := &Middleware{
		requestQueue: make(chan Request, queueCapacity),
		isLeaderUp:   false,
		validRegions: map[string]bool{
			"asia": true,
			"usa":  true,
		},
		currentLeader: -1,
	}

	go m.pollForLeader()
	go m.startOperationProcessor() // Start the rate-limited processor
	return m
}

// pollForLeader periodically polls nodes to find the current leader. (Improved version)
func (m *Middleware) pollForLeader() {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		leaderFound := false
		for i := 1; i <= max_Nodes; i++ {
			address := fmt.Sprintf("http://node-%d:%d/leader", i, nodeBasePort)
			resp, err := client.Get(address)
			if err != nil {
				// Node might be down, continue checking others
				// log.Printf("Error polling leader from node %d: %v", i, err) // Optional verbose logging
				continue
			}

			// Ensure body is closed even if scanning fails
			bodyBytes, readErr := ioutil.ReadAll(resp.Body)
			// It's crucial to close the body *before* checking readErr in this pattern
			resp.Body.Close()
			if readErr != nil {
				log.Printf("Error reading leader response body from node %d: %v", i, readErr)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				// log.Printf("Non-OK status %d polling leader from node %d", resp.StatusCode, i) // Optional verbose logging
				continue
			}

			var leaderID int
			// Try parsing formats: "Current leader: Node X (Term: Y)" or "Current leader: Node X"
			n, scanErr := fmt.Sscanf(string(bodyBytes), "Current leader: Node %d (Term: %*d)", &leaderID)
			if scanErr != nil || n != 1 {
				n, scanErr = fmt.Sscanf(string(bodyBytes), "Current leader: Node %d", &leaderID)
				if scanErr != nil || n != 1 {
					log.Printf("Error parsing leader response from node %d ('%s'): %v", i, string(bodyBytes), scanErr)
					continue
				}
			}

			// Validate leader ID
			if leaderID > 0 && leaderID <= max_Nodes {
				m.mutex.Lock()
				if m.currentLeader != leaderID {
					log.Printf("New leader detected: Node %d (Reported by Node %d)", leaderID, i)
				}
				m.currentLeader = leaderID
				m.isLeaderUp = true
				leaderFound = true
				m.mutex.Unlock()
				break // Found leader
			}
		}

		if !leaderFound {
			m.mutex.Lock()
			if m.isLeaderUp { // Log only if status changed
				log.Printf("No leader available, requests will be queued.")
			}
			m.isLeaderUp = false
			m.currentLeader = -1 // Reset leader ID
			m.mutex.Unlock()
		}

		time.Sleep(pollInterval)
	}
}

// processQueue processes requests from the queue, forwarding them to the leader. (Improved version)
// func (m *Middleware) processQueue() {
// 	for req := range m.requestQueue {
// 		m.mutex.RLock()
// 		leader := m.currentLeader
// 		isLeaderUp := m.isLeaderUp
// 		m.mutex.RUnlock()

// 		if !isLeaderUp || leader <= 0 {
// 			// Leader not available, re-queue or notify failure
// 			select {
// 			case m.requestQueue <- req:
// 				log.Printf("Re-queued request, waiting for leader.")
// 			default:
// 				log.Printf("Queue full while waiting for leader, request dropped.")
// 				select { // Non-blocking send on buffered done channel
// 				case req.done <- fmt.Errorf("service unavailable: leader down and queue full"):
// 				default:
// 				}
// 			}
// 			time.Sleep(500 * time.Millisecond) // Delay before next attempt
// 			continue
// 		}

// 		// Leader is available, forward the request
// 		targetURL, _ := url.Parse(fmt.Sprintf("http://node-%d:%d", leader, nodeBasePort))
// 		proxy := httputil.NewSingleHostReverseProxy(targetURL)

// 		// Use a timeout context for the forwarded request
// 		ctx, cancel := context.WithTimeout(req.r.Context(), 15*time.Second) // Increased forwarding timeout

// 		// Run forwarding in a goroutine
// 		go func(p *httputil.ReverseProxy, request Request, cancelFunc context.CancelFunc) {
// 			defer cancelFunc() // Ensure context is canceled
// 			// Forward the request with the timeout context
// 			err := m.forwardRequest(p, request.w, request.r.WithContext(ctx))
// 			select { // Non-blocking send
// 			case request.done <- err:
// 			default:
// 				log.Printf("Done channel full or receiver gone for request %s %s", request.r.Method, request.r.URL.Path)
// 			}
// 		}(proxy, req, cancel)
// 	}
// }

// Update forwardRequest around line 275
func (m *Middleware) forwardRequest(proxy *httputil.ReverseProxy, w http.ResponseWriter, r *http.Request) error {
	log.Printf("Forwarding request %s %s to leader Node %d", r.Method, r.URL.Path, m.currentLeader)

	// Create a custom response writer to capture status code
	captureWriter := &responseCapture{ResponseWriter: w}

	// ServeHTTP blocks until the response is written or an error occurs.
	proxy.ServeHTTP(captureWriter, r)

	// Check if the response was successful
	if captureWriter.statusCode >= 200 && captureWriter.statusCode < 300 {
		w.Header().Set("X-Response-Status", "success")
	} else {
		w.Header().Set("X-Response-Status", "error")
	}

	// Check the request context's error after ServeHTTP returns.
	if err := r.Context().Err(); err != nil {
		log.Printf("Forwarding failed for %s %s: context error: %v", r.Method, r.URL.Path, err)
		w.Header().Set("X-Response-Status", "error")
		return err
	}
	return nil
}

// Add this type at the top of your file
type responseCapture struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseCapture) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// ServeHTTP handles incoming HTTP requests *other than* /replication-summary.
func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract region from path (using user's original logic, slightly safer)
	pathParts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(pathParts) < 1 || pathParts[0] == "" { // Check if region exists
		http.Error(w, "Invalid path: Missing region", http.StatusBadRequest)
		return
	}
	region := pathParts[0]

	// Check if region is valid
	if !m.validRegions[region] {
		http.Error(w, fmt.Sprintf("Invalid region: %s", region), http.StatusBadRequest)
		return
	}

	if strings.HasSuffix(r.URL.Path, "/query") && r.Method == http.MethodPost {
		// Make a copy of the request body
		bodyBytes, _ := ioutil.ReadAll(r.Body)
		r.Body.Close()

		// Create a new reader with the same body for further processing
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

		// Try to parse the request to determine operation type
		var queryRequest struct {
			Type   string                 `json:"type"`
			Table  string                 `json:"table"`
			Values map[string]interface{} `json:"values"`
		}

		if err := json.Unmarshal(bodyBytes, &queryRequest); err == nil {
			// Record only if we can parse the request
			operationType := strings.ToLower(queryRequest.Type)
			if operationType == "insert" || operationType == "update" || operationType == "delete" {
				// Create an operation record
				operationTime := time.Now()
				operation := OperationRecord{
					Type:       operationType,
					Timestamp:  operationTime,
					Status:     "pending",
					Table:      queryRequest.Table,
					QueryType:  queryRequest.Type,
					Forwarded:  true,
					TargetNode: m.currentLeader,
				}

				// Extract email for user operations if present
				if email, ok := queryRequest.Values["email"].(string); ok {
					operation.Email = email
				}

				// Record the operation
				addOperation(operation)

				// Update the status when the request is processed
				defer func(opTime time.Time) {
					// Check if the headers contain a status code indicating success
					if w.Header().Get("X-Response-Status") == "success" {
						updateOperationStatus(opTime, "completed", "")
					} else {
						updateOperationStatus(opTime, "failed", "Request failed or node unavailable")
					}
				}(operationTime)
			}
		}

		// Create a new reader again for the actual processing
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Modify the request URL path for backend
	originalPath := r.URL.Path // Store for logging
	if len(pathParts) > 1 {
		r.URL.Path = "/" + pathParts[1] // e.g., /asia/query -> /query
	} else {
		r.URL.Path = "/" // Handle case like /asia/ -> /
	}

	// Add/Append X-Forwarded-For header
	if clientIP := r.Header.Get("X-Forwarded-For"); clientIP != "" {
		r.Header.Set("X-Forwarded-For", clientIP+", "+r.RemoteAddr)
	} else {
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	log.Printf("Received request for region %s: %s %s (original path: %s)", region, r.Method, r.URL.Path, originalPath)

	// Use buffered channel for done signal
	done := make(chan error, 1)
	req := Request{
		w:    w,
		r:    r,
		done: done,
	}

	// Try to queue the request with a timeout (using user's original 10s queue timeout)
	select {
	case m.requestQueue <- req:
		// Request queued. Wait for processing result or timeout.
		// Use a timeout slightly longer than the forwarding timeout.
		select {
		case err := <-done:
			if err != nil {
				log.Printf("Error processing request %s %s: %v", r.Method, originalPath, err)
				// Attempt to send error only if headers likely not written
				// Checking Header() map is a common heuristic
				if _, written := w.Header()["Date"]; !written {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
				}
			}
			// If err is nil, proxy handled the response.

		case <-time.After(20 * time.Second): // Timeout waiting for processing completion (longer than forwarding timeout)
			log.Printf("Timeout waiting for request processing completion: %s %s", r.Method, originalPath)
			if _, written := w.Header()["Date"]; !written {
				http.Error(w, "Request processing timeout", http.StatusGatewayTimeout)
			}
		}

	case <-time.After(10 * time.Second): // User's original timeout for *queueing*
		log.Printf("Request queue full or queueing timed out, dropping request: %s %s", r.Method, originalPath)
		http.Error(w, "Service busy or queue timeout", http.StatusServiceUnavailable)
	}
}

// --- Replication Status Structs and Functions (Added/Updated) ---

// NodeStatus holds the last log ID and timestamp for a node.
type NodeStatus struct {
	NodeId           int    `json:"nodeId"`
	LastLogId        int    `json:"lastLogId"`
	LastLogTimestamp string `json:"lastLogTimestamp,omitempty"` // Expect string from node
	Error            string `json:"error,omitempty"`
}

// ReplicationSummaryResponse is the structure for the /replication-summary endpoint.
type ReplicationSummaryResponse struct {
	Nodes map[string]NodeStatus `json:"nodes"` // Map NodeID (string) to its status
}

// fetchLogStatus queries a single node's /log-status endpoint.
func fetchLogStatus(nodeId int, nodeAddress string) NodeStatus {
	client := &http.Client{Timeout: 2 * time.Second} // Short timeout for status check
	url := fmt.Sprintf("http://%s/log-status", nodeAddress)
	status := NodeStatus{NodeId: nodeId} // Initialize with the expected nodeId

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error fetching status from %s (Node %d): %v", nodeAddress, nodeId, err)
		status.Error = err.Error()
		return status
	}
	defer resp.Body.Close()

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Printf("Error reading status body from %s (Node %d): %v", nodeAddress, nodeId, readErr)
		status.Error = fmt.Sprintf("read error: %v", readErr)
		return status
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))
		log.Printf("Non-OK response fetching status from %s (Node %d): %s", nodeAddress, nodeId, errMsg)
		status.Error = errMsg
		return status
	}

	// Decode the response: expected { "nodeId": X, "lastLogId": Y, "lastLogTimestamp": "..." }
	var receivedStatus NodeStatus
	if err := json.Unmarshal(body, &receivedStatus); err != nil {
		log.Printf("Error decoding status from %s (Node %d): %v", nodeAddress, nodeId, err)
		status.Error = fmt.Sprintf("decode error: %v", err)
		return status
	}

	// Return the successfully decoded status including the timestamp string
	return receivedStatus
}

// replicationSummaryHandler handles /replication-summary requests.
func (m *Middleware) replicationSummaryHandler(w http.ResponseWriter, r *http.Request) {
	var wg sync.WaitGroup
	results := make(map[string]NodeStatus)
	var mu sync.Mutex

	// Query all potential nodes concurrently
	for i := 1; i <= max_Nodes; i++ {
		wg.Add(1)
		go func(nodeID int) {
			defer wg.Done()
			address := fmt.Sprintf("node-%d:%d", nodeID, nodeBasePort)
			status := fetchLogStatus(nodeID, address) // This now includes timestamp

			mu.Lock()
			// Use node ID reported by the node itself if available, otherwise the loop ID
			keyNodeId := strconv.Itoa(status.NodeId)
			if status.NodeId == 0 { // Fallback if node didn't report its ID
				keyNodeId = strconv.Itoa(nodeID)
				status.NodeId = nodeID
			}
			results[keyNodeId] = status
			mu.Unlock()
		}(i)
	}

	wg.Wait() // Wait for all queries

	response := ReplicationSummaryResponse{Nodes: results}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding replication summary response: %v", err)
	}
}

// --- Node Control Handler (CORRECTED) ---

// handleNodeControl processes requests to start/stop node containers.
func handleNodeControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Expected path: /control/node/{id}/{action}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "control" || parts[1] != "node" {
		http.Error(w, "Invalid control path", http.StatusBadRequest)
		return
	}

	nodeIDStr := parts[2]
	action := parts[3]

	nodeID, err := strconv.Atoi(nodeIDStr)
	if err != nil || nodeID < 1 || nodeID > max_Nodes {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	if action != "start" && action != "stop" {
		http.Error(w, "Invalid action (use 'start' or 'stop')", http.StatusBadRequest)
		return
	}

	// Record the operation
	operationTime := time.Now()
	op := OperationRecord{
		Type:      action,
		NodeID:    nodeID,
		Timestamp: operationTime,
		Status:    "pending",
	}
	addOperation(op)

	// Use the explicit container name defined in docker-compose.yml
	containerName := fmt.Sprintf("node-%d", nodeID)

	log.Printf("Executing command: docker %s %s", action, containerName)

	// Execute the docker command
	cmd := exec.Command("docker", action, containerName)
	output, err := cmd.CombinedOutput() // Get stdout and stderr

	log.Printf("Command output: %s", string(output))

	response := map[string]string{}
	if err != nil {
		log.Printf("Error executing docker command: %v", err)
		response["status"] = "error"
		// Updated error message to show the target container name
		message := fmt.Sprintf("Failed to %s container '%s': %v. Output: %s", action, containerName, err, string(output))
		response["message"] = message
		w.WriteHeader(http.StatusInternalServerError)
		updateOperationStatus(operationTime, "failed", message)
	} else {
		response["status"] = "success"
		// Updated success message
		message := fmt.Sprintf("Container '%s' %s request sent successfully. Output: %s", containerName, action, string(output))
		response["message"] = message
		w.WriteHeader(http.StatusOK)
		updateOperationStatus(operationTime, "completed", "")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// --- End Node Control Handler ---

// handleGetCurrentLeader returns the leader ID currently known by the middleware.
func (m *Middleware) handleGetCurrentLeader(w http.ResponseWriter, r *http.Request) {
	m.mutex.RLock()
	// Use -1 or 0 to indicate no leader known, matching how it's initialized/set
	leader := m.currentLeader
	m.mutex.RUnlock()

	response := map[string]interface{}{
		"currentLeaderId": leader, // Will be -1 if no leader is currently known
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding current leader response: %v", err)
		// Can't reliably write http error if encoding fails late
	}
}

// --- End Current Leader Handler ---

func handleMiddlewareReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Record the operation
	operationTime := time.Now()
	op := OperationRecord{
		Type:      "reset",
		Timestamp: operationTime,
		Status:    "pending",
	}
	addOperation(op)

	// Get the membership host from environment variables
	membershipHost := os.Getenv("MEMBERSHIP_HOST")
	if membershipHost == "" {
		log.Printf("MEMBERSHIP_HOST environment variable not set")
		updateOperationStatus(operationTime, "failed", "MEMBERSHIP_HOST not set")
		http.Error(w, "Server configuration error: MEMBERSHIP_HOST not set", http.StatusInternalServerError)
		return
	}

	// Forward reset request to all nodes
	members, err := getMembershipList(membershipHost)
	if err != nil {
		log.Printf("Error getting membership list: %v", err)
		updateOperationStatus(operationTime, "failed", fmt.Sprintf("Failed to get membership list: %v", err))
		http.Error(w, fmt.Sprintf("Failed to get membership list: %v", err), http.StatusInternalServerError)
		return
	}

	if len(members) == 0 {
		log.Printf("No members found in membership list")
		updateOperationStatus(operationTime, "failed", "No members found in membership list")
		http.Error(w, "No members found in membership list", http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	errors := make(chan string, len(members))

	log.Printf("Attempting to reset %d nodes", len(members))
	updateOperationStatus(operationTime, "processing", fmt.Sprintf("Processing reset for %d nodes", len(members)))

	for _, member := range members {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			client := &http.Client{Timeout: 10 * time.Second}

			log.Printf("Sending reset request to %s", address)
			resp, err := client.Post(fmt.Sprintf("http://%s/reset", address), "application/json", nil)
			if err != nil {
				log.Printf("Error resetting node at %s: %v", address, err)
				errors <- fmt.Sprintf("Failed to reset node at %s: %v", address, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := ioutil.ReadAll(resp.Body)
				log.Printf("Node at %s responded with non-OK status %d: %s", address, resp.StatusCode, string(body))
				errors <- fmt.Sprintf("Node at %s responded with status %d: %s", address, resp.StatusCode, string(body))
			} else {
				log.Printf("Successfully reset node at %s", address)
			}
		}(member.Address)
	}

	// Wait for all reset operations to complete
	wg.Wait()
	close(errors)

	// Check for errors
	errorList := []string{}
	for err := range errors {
		errorList = append(errorList, err)
	}

	// Return appropriate response
	w.Header().Set("Content-Type", "application/json")
	if len(errorList) > 0 {
		if len(errorList) < len(members) {
			// Partial success
			updateOperationStatus(operationTime, "partial_success", fmt.Sprintf("%d errors occurred", len(errorList)))
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "partial_success",
				"message": "Some nodes were reset successfully, but errors occurred with others",
				"errors":  errorList,
			})
		} else {
			// Complete failure
			updateOperationStatus(operationTime, "failed", fmt.Sprintf("Failed to reset all %d nodes", len(members)))
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": "Failed to reset any nodes",
				"errors":  errorList,
			})
		}
	} else {
		// Complete success
		updateOperationStatus(operationTime, "completed", "All nodes reset successfully")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "All nodes reset successfully",
		})
	}
}

// Add this function with your other handlers
func handleOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for filtering
	queryType := r.URL.Query().Get("type")
	limit := 50 // Default limit

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100 // Cap at 100 for performance
			}
		}
	}

	// Get all operations
	operations := getOperationsQueue()

	// Filter by type if specified
	if queryType != "" {
		filtered := make([]OperationRecord, 0, len(operations))
		for _, op := range operations {
			if op.Type == queryType {
				filtered = append(filtered, op)
			}
		}
		operations = filtered
	}

	// Limit the number of operations returned
	if len(operations) > limit {
		operations = operations[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"operations": operations,
	})
}

// main function updated to use ServeMux and apply CORS correctly.
func main() {
	middleware := NewMiddleware()
	log.Printf("Starting middleware on port %d", middlewarePort)

	// --- Use ServeMux for Routing ---
	mux := http.NewServeMux()

	// Register the specific handler for replication status
	mux.HandleFunc("/replication-summary", middleware.replicationSummaryHandler)

	mux.HandleFunc("/control/node/", handleNodeControl) // Handles /control/node/1/start etc.

	mux.HandleFunc("/current-leader", middleware.handleGetCurrentLeader)

	mux.HandleFunc("/reset", handleMiddlewareReset)

	mux.HandleFunc("/operations", handleOperations)
	// Register the main middleware handler for all other paths (e.g., /asia/query)
	// The Middleware struct itself implements ServeHTTP for this purpose.
	mux.Handle("/", middleware)
	// --- End ServeMux Setup ---

	// Configure CORS options (using options similar to user's original intent)
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:8080", "http://localhost:8081", "*"}, // Include all origins
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},             // Common methods
		AllowedHeaders:   []string{"Content-Type", "Authorization", "*"},                  // Allow common headers + others
		AllowCredentials: true,
		Debug:            true, // Useful for troubleshooting
	})

	// Wrap the mux with the CORS handler
	handler := c.Handler(mux)

	// Start the server with the CORS-wrapped mux (using settings from user's original code)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", middlewarePort),
		Handler:      handler, // Use the CORS-wrapped mux
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Middleware listening on :%d", middlewarePort)
	log.Fatal(server.ListenAndServe())
}
