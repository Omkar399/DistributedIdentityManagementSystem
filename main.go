package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	basePort          = 8000
	httpPort          = 8080
	heartbeatInterval = 2 * time.Second
	leaderTimeout     = 4 * time.Second
)

type Node struct {
	ID              int
	Leader          bool
	lastKnownLeader int
	mutex           sync.RWMutex
	activeNodes     map[int]bool
	votes           map[int]bool
	term            int
	address         string
	membershipHost  string
}

type MemberInfo1 struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	LeaseID   int64     `json:"lease_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IsLeader  bool      `json:"is_leader"`
}

type Message struct {
	Type        string      // "VoteRequest" or "Heartbeat"
	VoteRequest VoteRequest // Used if Type is "VoteRequest"
	Heartbeat   struct {    // Used if Type is "Heartbeat"
		Term   int
		Leader int
	}
}

type SpanningTreeNode struct {
	ID       string
	address  string
	Parent   *SpanningTreeNode
	Children []*SpanningTreeNode
	mu       sync.RWMutex
}

type SpanningTree struct {
	Root *SpanningTreeNode
	mu   sync.RWMutex
}

type VoteRequest struct {
	CandidateID int
	Term        int
}

type VoteResponse struct {
	VoteGranted bool
	Term        int
}

func InitGlobalTree() {
	treeOnce.Do(func() {
		globalTree = &SpanningTree{
			Root: nil,
			mu:   sync.RWMutex{},
		}
	})
}

// GetGlobalTree returns the global tree, initializing it if necessary
func GetGlobalTree() *SpanningTree {
	InitGlobalTree()
	return globalTree
}

var (
	lastHeartbeat      time.Time
	heartbeatMutex     sync.RWMutex
	globalTree         *SpanningTree
	treeOnce           sync.Once
	prevMembershipList []string
	recovery           bool
)

func main() {
	nodeID, _ := strconv.Atoi(os.Getenv("NODE_ID"))
	membershipHost := os.Getenv("MEMBERSHIP_HOST")

	node := &Node{
		ID:             nodeID,
		activeNodes:    make(map[int]bool),
		votes:          make(map[int]bool),
		term:           0,
		address:        fmt.Sprintf("node-%d:8080", nodeID),
		membershipHost: membershipHost,
	}

	err := initDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Register with membership service
	if err := registerWithMembership(node); err != nil {
		log.Fatal(err)
	}

	go listenForHeartbeats(node)
	go startHTTPServer(node)

	// Monitor membership changes and update active nodes list dynamically
	go monitorMembershipChanges(node)

	go sendHeartbeatToMembership(node)

	time.Sleep(5 * time.Second)

	if !discoverExistingLeader(node) {
		startElection(node)
		recovery = false
	} else {
		recovery = true
		tree := GetGlobalTree()

		lastProcessedID, err := getLastProcessedID()
		if err != nil {
			log.Fatalf("Error reading last processed ID: %v\n", err)
		}

		fmt.Printf("Node starting with last processed ID %d\n", lastProcessedID)

		mem_list, _ := getMembershipList(membershipHost)
		leader, _ := GetLeaderNode(mem_list) // Replace with actual leader address
		fmt.Printf("leader : %d", leader.ID)
		tError := ConstructSpanningTree(tree, mem_list, leader.ID)
		if tError != nil {
			fmt.Printf("Error retrieving tree post recovery")
		}
		logs, err := requestMissingLogs(leader.Address, lastProcessedID)
		if err != nil {
			log.Fatalf("Error requesting missing logs: %v\n", err)
		}

		err = applyLogs(logs)
		if err != nil {
			log.Fatalf("Error applying logs: %v\n", err)
		}

		fmt.Println("Node synchronized successfully.")
	}

	for {
		if !isLeaderActive() {
			startElection(node)
		} else {
			recognizeLeader(node)
		}

		node.mutex.RLock()
		isLeader := node.Leader
		node.mutex.RUnlock()

		if isLeader {
			sendHeartbeats(node)
		}

		time.Sleep(heartbeatInterval)
	}
}

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

func GetLeaderNode(members map[string]*MemberInfo1) (*MemberInfo1, error) {
	for _, memberInfo := range members {
		if memberInfo.IsLeader {
			return memberInfo, nil
		}
	}
	return nil, fmt.Errorf("leader not found")
}

func registerWithMembership(node *Node) error {
	info := struct {
		ID      string `json:"id"`
		Address string `json:"address"`
	}{
		ID:      strconv.Itoa(node.ID),
		Address: node.address,
	}

	body, _ := json.Marshal(info)

	_, err := http.Post(
		fmt.Sprintf("http://%s/register", node.membershipHost),
		"application/json",
		bytes.NewBuffer(body),
	)

	return err
}

func sendHeartbeatToMembership(node *Node) {
	// Send periodic heartbeats to the membership service to indicate this node is alive.
	ticker := time.NewTicker(heartbeatInterval)
	for range ticker.C {
		info := struct {
			ID       string `json:"id"`
			Address  string `json:"address"`
			IsLeader bool   `json:"is_leader"`
		}{
			ID:       strconv.Itoa(node.ID),
			Address:  node.address,
			IsLeader: node.Leader,
		}
		body, _ := json.Marshal(info)
		http.Post(
			fmt.Sprintf("http://%s/keepalive", node.membershipHost),
			"application/json",
			bytes.NewBuffer(body),
		)
	}
}

func startElection(node *Node) {
	time.Sleep(time.Duration(150+rand.Intn(150)) * time.Millisecond)

	node.mutex.Lock()

	if node.lastKnownLeader > 0 && node.activeNodes[node.lastKnownLeader] {
		node.mutex.Unlock()
		return
	}

	if time.Since(lastHeartbeat) < leaderTimeout {
		node.mutex.Unlock()
		return
	}

	for i := 1; i < node.ID; i++ {
		if node.activeNodes[i] {
			node.mutex.Unlock()
			return
		}
	}

	node.term++
	currentTerm := node.term

	node.votes = make(map[int]bool)
	node.votes[node.ID] = true

	node.mutex.Unlock()

	votes := 1

	votingComplete := make(chan bool)

	go func() {
		time.Sleep(2 * time.Second)
		votingComplete <- true
	}()

	for id := range node.activeNodes {
		if id != node.ID {
			go func(targetID int) {
				if requestVote(node, targetID, currentTerm) {
					node.mutex.Lock()
					node.votes[targetID] = true
					votes++
					if votes >= len(node.activeNodes)/2+1 && !node.Leader {
						node.Leader = true
						node.lastKnownLeader = node.ID
						updateLastHeartbeat()
						votingComplete <- true
					}
					node.mutex.Unlock()
				}
			}(id)
		}
	}

	<-votingComplete
}

func requestVote(node *Node, targetID, term int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("node-%d:%d", targetID, basePort+targetID), time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	msg := Message{
		Type: "VoteRequest",
		VoteRequest: VoteRequest{
			CandidateID: node.ID,
			Term:        term,
		},
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		return false
	}

	var response VoteResponse
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&response); err != nil {
		return false
	}

	return response.VoteGranted
}

func listenForHeartbeats(node *Node) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", basePort+node.ID))
	if err != nil {
		log.Printf("Error starting listener: %v\n", err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		go handleConnection(node, conn)
	}
}

func handleConnection(node *Node, conn net.Conn) {
	defer conn.Close()

	var msg Message
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&msg); err != nil {
		return
	}

	node.mutex.Lock()
	defer node.mutex.Unlock()

	switch msg.Type {
	case "VoteRequest":
		response := VoteResponse{
			VoteGranted: false,
			Term:        node.term,
		}

		if msg.VoteRequest.Term > node.term ||
			(msg.VoteRequest.Term == node.term &&
				msg.VoteRequest.CandidateID < node.ID) {
			response.VoteGranted = true
			node.term = msg.VoteRequest.Term
			node.Leader = false
			node.lastKnownLeader = msg.VoteRequest.CandidateID
			updateLastHeartbeat()
		}

		encoder := json.NewEncoder(conn)
		encoder.Encode(response)

	case "Heartbeat":
		// Update term and leader if heartbeat is from current or newer term
		if msg.Heartbeat.Term >= node.term {
			node.term = msg.Heartbeat.Term
			node.Leader = false // This node is definitely not the leader
			node.lastKnownLeader = msg.Heartbeat.Leader
			updateLastHeartbeat()
		}
	}
}

func monitorMembershipChanges(node *Node) {
	ticker := time.NewTicker(heartbeatInterval)
	for range ticker.C {
		resp, err := http.Get(fmt.Sprintf("http://%s/members", node.membershipHost))
		if err != nil {
			continue
		}

		var members map[string]*MemberInfo1

		if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
			resp.Body.Close()
			continue
		}

		resp.Body.Close()

		node.mutex.Lock()

		for k := range node.activeNodes {
			delete(node.activeNodes, k)
		}

		for id, member := range members {
			nodeID, _ := strconv.Atoi(id)
			node.activeNodes[nodeID] = true

			if member.IsLeader {
				node.lastKnownLeader = nodeID
			}
		}

		node.mutex.Unlock()
	}
}

func recognizeLeader(node *Node) {
	node.mutex.Lock()
	defer node.mutex.Unlock()

	activeCount := 0
	for _, active := range node.activeNodes {
		if active {
			activeCount++
		}
	}

	// Calculate quorum size dynamically
	quorumSize := (activeCount / 2) + 1

	if activeCount < quorumSize {
		node.Leader = false
		return
	}

	if node.lastKnownLeader > 0 && node.activeNodes[node.lastKnownLeader] {
		node.Leader = (node.ID == node.lastKnownLeader)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func startHTTPServer(node *Node) {
	http.HandleFunc("/leader", func(w http.ResponseWriter, r *http.Request) {
		node.mutex.RLock()
		defer node.mutex.RUnlock()
		fmt.Fprintf(w, "Current leader: Node %d (Term: %d)\n", node.lastKnownLeader, node.term)
	})

	// Add this handler
	http.HandleFunc("/log-status", func(w http.ResponseWriter, r *http.Request) {
		// Call the new function from database.go
		lastID, lastTimestamp, err := getLatestLogEntry()
		if err != nil {
			log.Printf("Node %d: Error getting latest log entry: %v", node.ID, err)
			http.Error(w, "Failed to get log status", http.StatusInternalServerError)
			return
		}
		status := map[string]interface{}{
			"nodeId":           node.ID,
			"lastLogId":        lastID,
			"lastLogTimestamp": lastTimestamp.Format(time.RFC3339Nano), // Format timestamp as ISO 8601 string
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("Node %d: Error encoding log status response: %v", node.ID, err)
		}
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		node.mutex.RLock()
		defer node.mutex.RUnlock()
		status := struct {
			NodeID        int
			IsLeader      bool
			Term          int
			ActiveNodes   map[int]bool
			CurrentLeader int
		}{
			NodeID:        node.ID,
			IsLeader:      node.Leader,
			Term:          node.term,
			ActiveNodes:   node.activeNodes,
			CurrentLeader: node.lastKnownLeader,
		}
		json.NewEncoder(w).Encode(status)
	})

	http.HandleFunc("/query", handleQuery)

	http.HandleFunc("/recvMulticast", recvMulticast)

	http.HandleFunc("/getTreeFromLeader", GetTreeFromLeader)

	http.HandleFunc("/reset", handleReset)

	http.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		if !node.Leader {
			http.Error(w, "Only leader can serve logs", http.StatusForbidden)
			return
		}

		lastIDStr := r.URL.Query().Get("last_id")
		lastID, err := strconv.Atoi(lastIDStr)
		if err != nil {
			http.Error(w, "Invalid last_id parameter", http.StatusBadRequest)
			return
		}

		logs, err := getLogsAfter(lastID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error retrieving logs: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {

		node.mutex.RLock()
		defer node.mutex.RUnlock()

		// Node status metrics
		fmt.Fprintf(w, "# HELP node_status Node status (1 for leader, 0 for follower)\n")
		fmt.Fprintf(w, "# TYPE node_status gauge\n")
		fmt.Fprintf(w, "node_status{node_id=\"%d\"} %d\n", node.ID, boolToInt(node.Leader))

		if db != nil {
			query := `SELECT email, R1, R2, R3, R4 FROM users`
			rows, err := db.Query(query)
			if err != nil {
				log.Printf("Database query error: %v", err)
				return
			}
			defer rows.Close()

			fmt.Fprintf(w, "# HELP user_roles User role assignments by role\n")
			fmt.Fprintf(w, "# TYPE user_roles gauge\n")

			for rows.Next() {
				var email string
				var r1, r2, r3, r4 bool
				if err := rows.Scan(&email, &r1, &r2, &r3, &r4); err == nil {
					roles := fmt.Sprintf("r1=%d,r2=%d,r3=%d,r4=%d", boolToInt(r1), boolToInt(r2), boolToInt(r3), boolToInt(r4))
					fmt.Fprintf(w, "user_roles{node_id=\"%d\",email=\"%s\",roles=\"%s\"} 1\n", node.ID, email, roles)
				} else {
					log.Printf("Error scanning row: %v", err)
				}
			}
		} else {
			log.Println("Database connection is nil, skipping user metrics")
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		// Term metric
		fmt.Fprintf(w, "# HELP node_term Current term number (used in leader election)\n")
		fmt.Fprintf(w, "# TYPE node_term gauge\n")
		fmt.Fprintf(w, "node_term{node_id=\"%d\"} %d\n", node.ID, node.term)

		// Active nodes metric
		fmt.Fprintf(w, "# HELP active_nodes Number of active nodes in the cluster\n")
		fmt.Fprintf(w, "# TYPE active_nodes gauge\n")
		fmt.Fprintf(w, "active_nodes{node_id=\"%d\"} %d\n", node.ID, len(node.activeNodes))

		// Current leader metric
		fmt.Fprintf(w, "# HELP current_leader The ID of the current leader node\n")
		fmt.Fprintf(w, "# TYPE current_leader gauge\n")
		fmt.Fprintf(w, "current_leader{node_id=\"%d\"} %d\n", node.ID, node.lastKnownLeader)
	})

	fmt.Printf("Starting HTTP server on port %d\n", httpPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil); err != nil {
		fmt.Printf("Error starting HTTP server: %v\n", err)
	}
}

func discoverExistingLeader(node *Node) bool {
	for i := range node.activeNodes {
		if pingNode(i) {
			leader, term, err := askForLeader(i)
			if err == nil && leader > 0 {
				node.mutex.Lock()
				if term >= node.term {
					node.term = term
					node.Leader = (node.ID == leader)
					node.lastKnownLeader = leader
					updateLastHeartbeat()
				}
				node.mutex.Unlock()
				fmt.Printf("Node %d: Discovered existing leader: Node %d (Term: %d)\n", node.ID, leader, term)
				return true
			}
		}
	}
	return false
}

func askForLeader(nodeID int) (int, int, error) {
	resp, err := http.Get(fmt.Sprintf("http://node-%d:%d/leader", nodeID, httpPort))
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	var leader, term int
	_, err = fmt.Fscanf(resp.Body, "Current leader: Node %d (Term: %d)", &leader, &term)
	return leader, term, err
}

func isLeaderActive() bool {
	heartbeatMutex.RLock()
	defer heartbeatMutex.RUnlock()
	return time.Since(lastHeartbeat) < leaderTimeout
}

func updateLastHeartbeat() {
	heartbeatMutex.Lock()
	lastHeartbeat = time.Now()
	heartbeatMutex.Unlock()
}

func pingNode(id int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("node-%d:%d", id, basePort+id), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func sendHeartbeats(node *Node) {
	node.mutex.RLock()
	if !node.Leader {
		node.mutex.RUnlock()
		return // Early return if not leader
	}
	currentTerm := node.term
	nodeID := node.ID
	node.mutex.RUnlock()

	msg := Message{
		Type: "Heartbeat",
		Heartbeat: struct {
			Term   int
			Leader int
		}{
			Term:   currentTerm,
			Leader: nodeID,
		},
	}

	for i := range node.activeNodes {
		if i != nodeID {
			go func(targetID int) {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("node-%d:%d", targetID, basePort+targetID), time.Second)
				if err != nil {
					return
				}
				defer conn.Close()

				encoder := json.NewEncoder(conn)
				if err := encoder.Encode(msg); err != nil {
					return
				}

				fmt.Printf("Node %d: Sent heartbeat to Node %d (Term: %d)\n", nodeID, targetID, currentTerm)
			}(i)
		}
	}
}

// Add this to your import list if not already there
// "io/ioutil"

// Add this new handler function to multicast.go or main.go
func handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Start a transaction
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error starting transaction: %v", err), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Delete all users
	_, err = tx.Exec("DELETE FROM users")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error deleting users: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete all transaction logs
	_, err = tx.Exec("DELETE FROM transaction_log")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error deleting transaction logs: %v", err), http.StatusInternalServerError)
		return
	}

	// Reset sequence for transaction_log table
	// (This line is crucial for resetting the last log ID)
	_, err = tx.Exec("ALTER SEQUENCE transaction_log_id_seq RESTART WITH 1")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error resetting sequence: %v", err), http.StatusInternalServerError)
		return
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error committing transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "System reset successfully",
	})
}
