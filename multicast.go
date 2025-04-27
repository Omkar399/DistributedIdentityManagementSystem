package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type MulticastMessage struct {
	Query      string        `json:"query"`
	Args       []interface{} `json:"args"`
	PID        int           `json:"pid"`
	QueryType  QueryType     `json:"queryType"`
	Table      string        `json:"table"`
	SourceNode string        `json:"sourceNode"` // Add source tracking
	MessageID  string        `json:"messageId"`  // Add message ID for deduplication
}

// For tracking processed messages to avoid duplicates
var processedMessages sync.Map

// Constants for reliability
const (
	maxRetries       = 3
	retryDelay       = 500 * time.Millisecond
	httpTimeout      = 5 * time.Second
	multicastTimeout = 10 * time.Second
)

func multicast(query string, args []interface{}, nodeId string, table string, queryType QueryType) error {
	ctx, cancel := context.WithTimeout(context.Background(), multicastTimeout)
	defer cancel()

	// Get membership list with retries
	var members map[string]*MemberInfo1
	var e error
	for i := 0; i < maxRetries; i++ {
		members, e = getMembershipList(os.Getenv("MEMBERSHIP_HOST"))
		if e == nil {
			break
		}
		fmt.Printf("Attempt %d: Failed to get membership list: %v\n", i+1, e)
		time.Sleep(retryDelay)
	}
	if e != nil {
		return fmt.Errorf("failed to query membership list after %d attempts: %v", maxRetries, e)
	}

	membersList := make([]string, 0, len(members))
	for k := range members {
		membersList = append(membersList, k)
	}

	// Get leader with retries
	var leader *MemberInfo1
	for i := 0; i < maxRetries; i++ {
		leader, e = GetLeaderNode(members)
		if e == nil {
			break
		}
		fmt.Printf("Attempt %d: Failed to get leader: %v\n", i+1, e)
		time.Sleep(retryDelay)
	}
	if e != nil {
		return fmt.Errorf("failed to get leader after %d attempts: %v", maxRetries, e)
	}

	fmt.Printf("prev list %v \n", prevMembershipList)
	fmt.Printf("curr list %v \n", membersList)

	// Get tree with proper locking
	tree := GetGlobalTree()

	// Initialize or update tree
	if tree.Root == nil {
		fmt.Println("Constructing new spanning tree")
		err := ConstructSpanningTree(tree, members, leader.ID)
		if err != nil {
			return fmt.Errorf("failed to construct spanning tree: %v", err)
		}
	} else {
		// Handle nodes leaving
		for _, prevMember := range prevMembershipList {
			if members[prevMember] == nil {
				// Delete all the nodes that have died or left the cluster
				fmt.Printf("Remove node : %s\n", prevMember)
				tree.RemoveNode(prevMember, leader.ID)
			}
		}

		sort.Strings(prevMembershipList)
		// Handle nodes joining
		for member := range members {
			index := sort.SearchStrings(prevMembershipList, member)
			found := index < len(prevMembershipList) && prevMembershipList[index] == member
			fmt.Printf("found : %v , %s", found, member)
			if found != true {
				fmt.Printf("Add node : %s\n", member)
				tree.AddNode(member, members[member].Address, leader.ID)
			}
		}
	}

	// Print the tree
	tree.PrintTree()

	// Update membership list for next time
	prevMembershipList = membersList
	fmt.Printf("Inside Multicast\n")

	// Get last processed ID with retries
	var lastProcessedId int
	var err error
	for i := 0; i < maxRetries; i++ {
		lastProcessedId, err = getLastProcessedID()
		if err == nil {
			break
		}
		fmt.Printf("Attempt %d: Failed to get last processed ID: %v\n", i+1, err)
		time.Sleep(retryDelay)
	}
	if err != nil {
		fmt.Printf("Failed to retrieve last processed Transaction after %d attempts\n", maxRetries)
		return fmt.Errorf("failed to get last processed ID: %v", err)
	}

	// Create unique message ID to prevent loops
	messageID := fmt.Sprintf("%s-%d-%s-%d", nodeId, lastProcessedId, table, time.Now().UnixNano())

	// Mark this message as processed by us
	processedMessages.Store(messageID, true)

	// Create message with source tracking
	msg := MulticastMessage{
		Query:      query,
		Args:       args,
		PID:        lastProcessedId,
		QueryType:  queryType,
		Table:      table,
		SourceNode: nodeId,
		MessageID:  messageID,
	}

	fmt.Printf("Multicasting node : %s\n", nodeId)
	multicastNode := tree.Root.FindNodeDFS(nodeId)
	if multicastNode == nil {
		return fmt.Errorf("node %s not found in tree", nodeId)
	}

	return multicastToChildrenWithRetry(ctx, multicastNode, msg)
}

func multicastToChildrenWithRetry(ctx context.Context, node *SpanningTreeNode, msg MulticastMessage) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	failCount := 0

	// Safe children access
	node.mu.RLock()
	children := make([]*SpanningTreeNode, 0, len(node.Children))
	for _, child := range node.Children {
		if child != nil {
			children = append(children, child)
		}
	}
	node.mu.RUnlock()

	// No children - nothing to do
	if len(children) == 0 {
		return nil
	}

	// Multicast to each child with retries
	for _, child := range children {
		wg.Add(1)
		go func(childNode *SpanningTreeNode) {
			defer wg.Done()

			// Try multiple times with backoff
			var err error
			for i := 0; i < maxRetries; i++ {
				select {
				case <-ctx.Done():
					// Context timeout or cancellation
					mu.Lock()
					failCount++
					mu.Unlock()
					return
				default:
					err = sendMulticast(childNode.address, msg)
					if err == nil {
						return // Success
					}
					fmt.Printf("Retry %d: Failed to multicast to %s: %v\n", i+1, childNode.ID, err)
					time.Sleep(retryDelay)
				}
			}

			// All retries failed
			if err != nil {
				mu.Lock()
				failCount++
				fmt.Printf("All retries failed for node %s: %v\n", childNode.ID, err)
				mu.Unlock()
			}
		}(child)
	}

	// Wait for all multicasts to complete
	wg.Wait()

	// Continue if at least one child succeeded
	if failCount == len(children) && len(children) > 0 {
		return fmt.Errorf("multicast failed to all %d children", len(children))
	} else if failCount > 0 {
		fmt.Printf("Warning: multicast partially failed (%d of %d nodes unreachable)\n",
			failCount, len(children))
	}

	return nil
}

func sendMulticast(address string, msg MulticastMessage) error {
	fmt.Println("Send Multicast")
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal multicast message: %v", err)
	}

	fmt.Printf("http://%s/recvMulticast", address)
	fmt.Println(bytes.NewBuffer(jsonData))

	client := &http.Client{
		Timeout: httpTimeout,
	}

	resp, err := client.Post(fmt.Sprintf("http://%s/recvMulticast", address),
		"application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send multicast: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("multicast failed with status: %s", resp.Status)
	}

	return nil
}

func recvMulticast(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received Multicast\n")
	var msg MulticastMessage

	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		fmt.Printf("Error decoding multicast message: %v\n", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Check for duplicate messages
	if _, seen := processedMessages.LoadOrStore(msg.MessageID, true); seen {
		fmt.Printf("Ignoring duplicate message: %s\n", msg.MessageID)
		w.WriteHeader(http.StatusOK) // Still return OK
		return
	}

	// Don't process messages from ourselves
	nodeID := os.Getenv("NODE_ID")
	if msg.SourceNode == nodeID {
		fmt.Printf("Ignoring message from self\n")
		w.WriteHeader(http.StatusOK)
		return
	}

	lastProcessedId, err := getLastProcessedID()
	if err != nil {
		fmt.Printf("Error getting last processed ID: %v\n", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	fmt.Printf("Last Processed id : %d msg.PID : %d\n", lastProcessedId, msg.PID)
	if lastProcessedId+1 != msg.PID {
		fmt.Printf("Multicast missed -> Syncing data\n")

		// Get membership list with retry
		var mem_list map[string]*MemberInfo1
		var leaderErr error
		for i := 0; i < maxRetries; i++ {
			mem_list, err = getMembershipList(os.Getenv("MEMBERSHIP_HOST"))
			if err == nil {
				break
			}
			fmt.Printf("Attempt %d: Failed to get membership list for sync: %v\n", i+1, err)
			time.Sleep(retryDelay)
		}
		if err != nil {
			fmt.Printf("Failed to get membership list after %d attempts\n", maxRetries)
			http.Error(w, "Failed to sync: cannot get membership list", http.StatusInternalServerError)
			return
		}

		// Get leader with retry
		var leader *MemberInfo1
		for i := 0; i < maxRetries; i++ {
			leader, leaderErr = GetLeaderNode(mem_list)
			if leaderErr == nil {
				break
			}
			fmt.Printf("Attempt %d: Failed to get leader for sync: %v\n", i+1, leaderErr)
			time.Sleep(retryDelay)
		}
		if leaderErr != nil {
			fmt.Printf("Failed to get leader after %d attempts\n", maxRetries)
			http.Error(w, "Failed to sync: cannot determine leader", http.StatusInternalServerError)
			return
		}

		// Request missing logs with retry - Using correct type []map[string]interface{}
		var logs []map[string]interface{}
		for i := 0; i < maxRetries; i++ {
			logs, err = requestMissingLogs(leader.Address, lastProcessedId)
			if err == nil {
				break
			}
			fmt.Printf("Attempt %d: Failed to request missing logs: %v\n", i+1, err)
			time.Sleep(retryDelay)
		}
		if err != nil {
			fmt.Printf("Error requesting missing logs after %d attempts: %v\n", maxRetries, err)
			http.Error(w, "Failed to request logs", http.StatusInternalServerError)
			return
		}

		// Apply logs with retry
		for i := 0; i < maxRetries; i++ {
			err = applyLogs(logs)
			if err == nil {
				break
			}
			fmt.Printf("Attempt %d: Failed to apply logs: %v\n", i+1, err)
			time.Sleep(retryDelay)
		}
		if err != nil {
			fmt.Printf("Error applying logs after %d attempts: %v\n", maxRetries, err)
			http.Error(w, "Failed to apply logs", http.StatusInternalServerError)
			return
		}

		fmt.Println("Node synchronized successfully.")

		// Verify synchronization
		lastProcessedId, err = getLastProcessedID()
		if err != nil {
			fmt.Printf("Error verifying sync: %v\n", err)
			http.Error(w, "Failed to verify sync", http.StatusInternalServerError)
			return
		}

		// If still out of sync after sync attempt, reject the message
		if lastProcessedId+1 != msg.PID {
			fmt.Printf("Node still out of sync after recovery: %d vs %d\n", lastProcessedId, msg.PID)
			http.Error(w, "Node still out of sync after recovery", http.StatusInternalServerError)
			return
		}
	}

	// Execute the query with proper error handling
	rows, err := db.Query(msg.Query, msg.Args...)
	if err != nil {
		fmt.Printf("Error executing query: %v\n", err)
		http.Error(w, fmt.Sprintf("Error executing query: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Log the transaction
	err = logTransaction(msg.QueryType, msg.Table, msg.Query, msg.Args...)
	if err != nil {
		fmt.Printf("Error logging transaction: %v\n", err)
		// Continue despite logging error
	}

	fmt.Printf("Received Multicast Message: Query = %s, Args = %v\n", msg.Query, msg.Args)

	// Send success response first
	w.WriteHeader(http.StatusOK)

	// Forward multicast in background to avoid blocking
	go func() {
		if err := multicast(msg.Query, msg.Args, nodeID, msg.Table, msg.QueryType); err != nil {
			fmt.Printf("Error forwarding multicast: %v\n", err)
		}
	}()
}
