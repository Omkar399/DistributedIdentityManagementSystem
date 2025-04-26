package main

import (
	"database/sql"
	"encoding/json"
	"errors" // Import errors package for errors.As
	"fmt"
	"io" // Import io for io.EOF
	"io/ioutil"
	"log" // Use log package for logging
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt" // Import bcrypt

	_ "github.com/lib/pq" // PostgreSQL driver
)

// QueryType defines the type of SQL query.
type QueryType string

const (
	QueryTypeSelect QueryType = "SELECT"
	QueryTypeInsert QueryType = "INSERT"
	QueryTypeUpdate QueryType = "UPDATE"
	QueryTypeDelete QueryType = "DELETE"
)

// QueryRequest represents the structure of a JSON query request from the middleware.
type QueryRequest struct {
	Type      QueryType         `json:"type"`
	Table     string            `json:"table"`
	Fields    []string          `json:"fields,omitempty"`
	Where     map[string]string `json:"where,omitempty"`
	Values    map[string]string `json:"values,omitempty"` // Frontend sends string values
	DeleteAll bool              `json:"delete_all,omitempty"`
}

var db *sql.DB // Global database connection pool

// initDB initializes the database connection and ensures tables exist.
func initDB() error {
	var err error
	// Construct connection string using environment variables
	dbHost := os.Getenv("DB_HOST") // Use DB_HOST set in docker-compose [2]
	if dbHost == "" {
		return fmt.Errorf("DB_HOST environment variable not set")
	}
	connStr := fmt.Sprintf("host=%s user=postgres password=password dbname=nodedb sslmode=disable", dbHost)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("error connecting to database: %v", err)
	}
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("error pinging database: %v", err)
	}
	log.Printf("Node %s successfully connected to database on host %s\n", os.Getenv("NODE_ID"), dbHost)

	// Check and Create users table
	var tableExists bool
	err = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'users')").Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("error checking if users table exists: %v", err)
	}

	if !tableExists {
		// --- Updated Schema ---
		_, err = db.Exec(`
			CREATE TABLE users (
				email VARCHAR(255) PRIMARY KEY,
				password_hash TEXT NOT NULL, -- Store bcrypt hash (includes salt) [7]
				R1 BOOLEAN,
				R2 BOOLEAN,
				R3 BOOLEAN,
				R4 BOOLEAN
			)
		`)
		// --- End Updated Schema ---
		if err != nil {
			return fmt.Errorf("error creating users table: %v", err)
		}
		log.Println("Created users table")
	} else {
		log.Println("Users table already exists")
		// Optional: Add ALTER TABLE logic here if needed to modify existing tables
	}

	// Check and Create transaction_log table
	var logTableExists bool
	err = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'transaction_log')").Scan(&logTableExists)
	if err != nil {
		return fmt.Errorf("error checking if transaction_log table exists: %v", err)
	}

	if !logTableExists {
		_, err = db.Exec(`
			CREATE TABLE transaction_log (
				id SERIAL PRIMARY KEY,
				type VARCHAR(10),
				table_name VARCHAR(255),
				query TEXT,
				timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)
		`)
		if err != nil {
			return fmt.Errorf("error creating transaction_log table: %v", err)
		}
		log.Println("Created transaction_log table")
	} else {
		log.Println("Transaction_log table already exists")
	}

	return nil
}

// --- Password Hashing Function ---
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost) // Use default cost [7][10]
	return string(bytes), err
}

// --- Password Checking Function (Example for future login implementation) ---
func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil // Returns true if password matches hash [7][10]
}

// logTransaction inserts a record of the executed query into the transaction_log table.
func logTransaction(queryType QueryType, table string, query string, params ...interface{}) error {
	// Format the query with its parameters for logging
	formattedQuery := formatQueryWithParams(query, params)

	// Log the formatted query
	_, err := db.Exec("INSERT INTO transaction_log (type, table_name, query) VALUES ($1, $2, $3)", queryType, table, formattedQuery)
	if err != nil {
		log.Printf("Error logging transaction: %v", err) // Log the error
	}
	return err
}

// formatQueryWithParams formats a SQL query string with its parameters for logging purposes.
// Warning: This is simplified; complex types might not format perfectly. Not safe for execution.
func formatQueryWithParams(query string, params []interface{}) string {
	for i, param := range params {
		placeholder := fmt.Sprintf("$%d", i+1)
		var value string
		switch v := param.(type) {
		case string:
			// Basic escaping for single quotes in strings for logging
			escapedString := strings.ReplaceAll(v, "'", "''")
			value = fmt.Sprintf("'%s'", escapedString)
		case bool:
			value = fmt.Sprintf("%t", v)
		case nil:
			value = "NULL"
		default:
			// Convert other types to string representation
			value = fmt.Sprintf("%v", v)
		}
		// Replace only the first occurrence of the placeholder in each iteration
		query = strings.Replace(query, placeholder, value, 1)
	}
	return query
}

// requestMissingLogs fetches logs from the leader node that occurred after the given lastID.
func requestMissingLogs(leaderAddress string, lastID int) ([]map[string]interface{}, error) {
	requestURL := fmt.Sprintf("http://%s/logs?last_id=%d", leaderAddress, lastID)
	log.Printf("Requesting logs from %s\n", requestURL) // Use log for consistency

	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("error requesting logs from %s: %v", leaderAddress, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("error response from leader %s (%s): %s", leaderAddress, resp.Status, string(bodyBytes))
	}

	var logs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, fmt.Errorf("error decoding logs response from %s: %v", leaderAddress, err)
	}

	log.Printf("Received %d log entries from leader %s\n", len(logs), leaderAddress)
	return logs, nil
}

// getLogsAfter retrieves logs from the local database after a specific ID.
func getLogsAfter(lastID int) ([]map[string]interface{}, error) {
	query := "SELECT id, type, table_name, query FROM transaction_log WHERE id > $1 ORDER BY id ASC"

	rows, err := db.Query(query, lastID)
	if err != nil {
		return nil, fmt.Errorf("error querying transaction logs: %v", err)
	}
	defer rows.Close()

	return scanRowsToMap(rows) // Use helper function
}

// applyLogs executes a list of log entries (SQL queries) against the local database within a transaction.
func applyLogs(logs []map[string]interface{}) error {
	tx, err := db.Begin() // Start a transaction
	if err != nil {
		return fmt.Errorf("failed to begin transaction for applying logs: %v", err)
	}
	defer tx.Rollback() // Rollback if anything fails

	for _, logEntry := range logs {
		query, okQuery := logEntry["query"].(string)
		logType, okType := logEntry["type"].(string)
		tableName, okTable := logEntry["table_name"].(string) // Get table name from log

		if !okQuery || !okType || !okTable {
			log.Printf("Skipping invalid log entry: %+v", logEntry) // Log invalid entry
			continue                                                // Skip malformed entries
		}

		log.Printf("Applying log entry: [%s] %s\n", logType, query)

		// 1. Insert the log entry itself into the local transaction log
		// Use the original query string from the log entry for consistency
		_, err := tx.Exec("INSERT INTO transaction_log (type, table_name, query) VALUES ($1, $2, $3)",
			logType, tableName, query)
		if err != nil {
			// If inserting the log fails (e.g., duplicate due to race condition), log it but maybe continue
			log.Printf("Warning: Failed to insert log entry locally (may already exist): %v", err)
			// Decide if this should halt the process or just be logged. Let's continue for now.
		}

		// 2. Execute the actual query from the log entry
		if _, err := tx.Exec(query); err != nil {
			// If applying the actual query fails, rollback is critical
			return fmt.Errorf("error applying log entry query '%s': %v", query, err)
		}
	}

	return tx.Commit() // Commit the transaction if all logs applied successfully
}

// getLastProcessedID retrieves the ID of the most recent entry in the local transaction log.
func getLastProcessedID() (int, error) {
	var lastID int
	// Use COALESCE to handle the case where the table is empty
	err := db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM transaction_log").Scan(&lastID)
	if err != nil && err != sql.ErrNoRows { // Allow ErrNoRows, which COALESCE handles
		return 0, fmt.Errorf("error retrieving last processed ID: %v", err)
	}
	// If ErrNoRows occurred but was handled by COALESCE, lastID will be 0, which is correct.
	return lastID, nil
}

func getLatestLogEntry() (int, time.Time, error) {
	var lastID int
	var lastTimestamp time.Time // Use time.Time type

	// Query for the latest entry by ordering by ID descending
	query := "SELECT id, timestamp FROM transaction_log ORDER BY id DESC LIMIT 1"
	err := db.QueryRow(query).Scan(&lastID, &lastTimestamp)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Table is empty, return 0 and zero time, not an error
			return 0, time.Time{}, nil
		}
		// Actual database error
		return 0, time.Time{}, fmt.Errorf("error retrieving latest log entry: %v", err)
	}

	return lastID, lastTimestamp, nil
}

// handleQuery processes incoming query requests from the middleware.
func handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size to prevent potential DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024) // 1 MB limit

	var queryRequest QueryRequest
	// Use DisallowUnknownFields to catch unexpected JSON fields
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&queryRequest)

	if err != nil {
		// Provide more specific error messages based on the error type
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var maxBytesError *http.MaxBytesError

		switch {
		case errors.Is(err, io.EOF): // Use errors.Is for EOF check
			http.Error(w, "Request body must not be empty", http.StatusBadRequest)
		case errors.As(err, &syntaxError):
			msg := fmt.Sprintf("Request body contains badly-formed JSON (at character %d)", syntaxError.Offset)
			http.Error(w, msg, http.StatusBadRequest)
		case errors.As(err, &unmarshalTypeError):
			msg := fmt.Sprintf("Request body contains an invalid value for the %q field (at character %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset)
			http.Error(w, msg, http.StatusBadRequest)
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			msg := fmt.Sprintf("Request body contains unknown field %s", fieldName)
			http.Error(w, msg, http.StatusBadRequest)
		case errors.As(err, &maxBytesError):
			msg := fmt.Sprintf("Request body must not be larger than %d bytes", maxBytesError.Limit)
			http.Error(w, msg, http.StatusRequestEntityTooLarge)
		default:
			log.Printf("Error decoding JSON: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest) // Generic error for other cases
		}
		return
	}

	if err := validateQueryRequest(queryRequest); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// --- Password Hashing Logic ---
	// If it's an INSERT query for the 'users' table, hash the password
	if queryRequest.Type == QueryTypeInsert && queryRequest.Table == "users" {
		plainPassword, ok := queryRequest.Values["password"]
		if !ok || plainPassword == "" {
			http.Error(w, "Password is required for user creation", http.StatusBadRequest)
			return
		}
		if len(plainPassword) > 72 { // Bcrypt has a max password length of 72 bytes [7][8]
			http.Error(w, "Password exceeds maximum length (72 bytes)", http.StatusBadRequest)
			return
		}

		hashedPassword, err := hashPassword(plainPassword)
		if err != nil {
			log.Printf("Error hashing password for user %s: %v", queryRequest.Values["email"], err)
			http.Error(w, "Internal server error processing password", http.StatusInternalServerError)
			return
		}

		// Replace plain password with hash and update the key to match the DB column
		delete(queryRequest.Values, "password")
		queryRequest.Values["password_hash"] = hashedPassword
		log.Printf("Password hashed successfully for user %s", queryRequest.Values["email"])
	}
	// --- End Password Hashing Logic ---

	var query string
	var args []interface{}
	var txErr error // To capture potential transaction errors

	// Build the query. Logging happens *after* potential password hashing.
	switch queryRequest.Type {
	case QueryTypeSelect:
		query, args = buildSelectQuery(queryRequest)
		// SELECT queries are not logged in the transaction log to avoid infinite loops during recovery
	case QueryTypeInsert:
		query, args = buildInsertQuery(queryRequest)
		// Log transaction *before* execution but *after* hashing
		txErr = logTransaction(QueryTypeInsert, queryRequest.Table, query, args...)
	case QueryTypeUpdate:
		query, args = buildUpdateQuery(queryRequest)
		// Log transaction *before* execution
		txErr = logTransaction(QueryTypeUpdate, queryRequest.Table, query, args...)
	case QueryTypeDelete:
		query, args = buildDeleteQuery(queryRequest)
		// Log transaction *before* execution
		txErr = logTransaction(QueryTypeDelete, queryRequest.Table, query, args...)
	default:
		// This case should ideally be caught by validation, but added for safety
		http.Error(w, "Invalid query type", http.StatusBadRequest)
		return
	}

	// If logging the transaction failed, return an error before executing
	if txErr != nil {
		http.Error(w, fmt.Sprintf("Failed to log transaction: %v", txErr), http.StatusInternalServerError)
		return
	}

	// Debug logging
	log.Printf("Executing query: %s\nWith args: %v\n", query, args)

	// Handle SELECT queries (no transaction needed, read-only)
	if queryRequest.Type == QueryTypeSelect {
		rows, err := db.Query(query, args...)
		if err != nil {
			log.Printf("Error executing SELECT query: %v", err)
			http.Error(w, fmt.Sprintf("Error executing query: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// Generic row scanning into map slice
		results, err := scanRowsToMap(rows)
		if err != nil {
			log.Printf("Error scanning rows: %v", err)
			http.Error(w, fmt.Sprintf("Error processing results: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results) // Send results back
		return
	}

	// Handle INSERT, UPDATE, DELETE queries (these should be multicasted if leader)

	// Multicast the query if this node is the leader (assuming multicast handles this check)
	// Multicasting the query *after* hashing and *before* local execution
	// The arguments 'args' now contain the hashed password if it was an insert
	// Multicast only write operations
	log.Printf("Multicasting query type: %s\n", queryRequest.Type)
	go func() {
		// Pass the final query and arguments (including potential hash)
		mcErr := multicast(query, args, os.Getenv("NODE_ID"), queryRequest.Table, queryRequest.Type)
		if mcErr != nil {
			log.Printf("Error during multicast: %v", mcErr) // Log multicast errors
		}
	}()

	// Execute the query locally
	result, err := db.Exec(query, args...)
	if err != nil {
		log.Printf("Error executing %s query: %v", queryRequest.Type, err)
		// Consider more specific error handling (e.g., unique constraint violations)
		http.Error(w, fmt.Sprintf("Error executing query: %v", err), http.StatusInternalServerError)
		return
	}

	rowCount, err := result.RowsAffected()
	if err != nil {
		// This error is less critical but should be logged
		log.Printf("Error fetching rows affected: %v", err)
		// Continue to return success response even if RowsAffected fails
		rowCount = 0 // Indicate uncertainty or zero rows affected
	}

	// Return a success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Explicitly set status code
	response := map[string]interface{}{
		"message":       "Query executed successfully",
		"rows_affected": rowCount,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// If encoding the response fails, log it, but headers might already be sent
		log.Printf("Error encoding success response: %v", err)
	}
}

// Helper function to scan rows into a slice of maps.
func scanRowsToMap(rows *sql.Rows) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error getting columns: %v", err)
	}
	// columnTypes variable removed as it was unused

	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		err := rows.Scan(pointers...)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %v", err)
		}

		row := make(map[string]interface{})
		for i, column := range columns {
			val := values[i]

			// Convert byte slices to strings, handle NULLs
			if b, ok := val.([]byte); ok {
				row[column] = string(b)
			} else if val == nil {
				row[column] = nil // Explicitly handle SQL NULL
			} else {
				row[column] = val // Assign other types directly
			}
		}
		results = append(results, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %v", err)
	}
	return results, nil
}

// validateQueryRequest performs basic validation on the query request structure.
func validateQueryRequest(req QueryRequest) error {
	if req.Table == "" {
		return fmt.Errorf("table name is required")
	}

	// Basic table name validation (prevent simple SQL injection attempts)
	// A more robust approach might use a whitelist of allowed table names.
	if strings.ContainsAny(req.Table, ";'\" ") {
		return fmt.Errorf("invalid table name")
	}

	switch req.Type {
	case QueryTypeSelect:
		if len(req.Fields) == 0 {
			return fmt.Errorf("at least one field is required for SELECT")
		}
		// Basic field name validation
		for _, field := range req.Fields {
			if strings.ContainsAny(field, ";'\" ") && field != "*" {
				return fmt.Errorf("invalid field name: %s", field)
			}
		}
	case QueryTypeInsert:
		if len(req.Values) == 0 {
			return fmt.Errorf("values are required for INSERT")
		}
		// Basic column name validation in values
		for col := range req.Values {
			if strings.ContainsAny(col, ";'\" ") {
				return fmt.Errorf("invalid column name in values: %s", col)
			}
		}
	case QueryTypeUpdate:
		if len(req.Values) == 0 {
			return fmt.Errorf("values are required for UPDATE")
		}
		if len(req.Where) == 0 { // Require WHERE for safety in UPDATE
			return fmt.Errorf("where clause is required for UPDATE")
		}
		// Basic column name validation
		for col := range req.Values {
			if strings.ContainsAny(col, ";'\" ") {
				return fmt.Errorf("invalid column name in values: %s", col)
			}
		}
		for col := range req.Where {
			if strings.ContainsAny(col, ";'\" ") {
				return fmt.Errorf("invalid column name in where clause: %s", col)
			}
		}
	case QueryTypeDelete:
		// Allow delete without WHERE only if explicitly requested via DeleteAll flag
		if len(req.Where) == 0 && !req.DeleteAll {
			return fmt.Errorf("where clause or explicit 'delete_all' flag is required for DELETE")
		}
		// Basic column name validation in where
		for col := range req.Where {
			if strings.ContainsAny(col, ";'\" ") {
				return fmt.Errorf("invalid column name in where clause: %s", col)
			}
		}
	default:
		return fmt.Errorf("invalid query type: %s", req.Type)
	}
	return nil
}

// buildSelectQuery constructs a SELECT SQL query string and its arguments.
func buildSelectQuery(req QueryRequest) (string, []interface{}) {
	// Sanitize field names minimally
	safeFields := []string{}
	for _, f := range req.Fields {
		if f == "*" || !strings.ContainsAny(f, ";'\" ") { // Allow '*' or simple names
			safeFields = append(safeFields, f)
		} else {
			log.Printf("Warning: Invalid field name skipped in SELECT: %s", f)
		}
	}
	if len(safeFields) == 0 {
		safeFields = append(safeFields, "*") // Default if all fields invalid
	}

	// Basic table name sanitization
	safeTable := req.Table
	if strings.ContainsAny(safeTable, ";'\" ") {
		log.Printf("Error: Invalid table name used in buildSelectQuery: %s", safeTable)
		return "", nil
	}

	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(safeFields, ", "), safeTable)
	var args []interface{}
	if len(req.Where) > 0 {
		whereClause, whereArgs := buildWhereClause(req.Where)
		if whereClause != "" {
			query += " WHERE " + whereClause
			args = whereArgs
		}
	}
	return query, args
}

// buildInsertQuery constructs an INSERT SQL query string and its arguments.
func buildInsertQuery(req QueryRequest) (string, []interface{}) {
	var columns, placeholders []string
	var args []interface{}
	i := 1

	// Basic table name sanitization
	safeTable := req.Table
	if strings.ContainsAny(safeTable, ";'\" ") {
		log.Printf("Error: Invalid table name used in buildInsertQuery: %s", safeTable)
		return "", nil
	}

	for col, val := range req.Values {
		// Basic column name sanitization
		if !strings.ContainsAny(col, ";'\" ") {
			columns = append(columns, col)
			placeholders = append(placeholders, fmt.Sprintf("$%d", i))
			args = append(args, val) // Value is used as parameter, safe from injection here
			i++
		} else {
			log.Printf("Warning: Invalid column name skipped in INSERT: %s", col)
		}
	}

	if len(columns) == 0 {
		log.Printf("Error: No valid columns found for INSERT into %s", safeTable)
		return "", nil
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		safeTable, strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	return query, args
}

// buildUpdateQuery constructs an UPDATE SQL query string and its arguments.
func buildUpdateQuery(req QueryRequest) (string, []interface{}) {
	var setClauses []string
	var args []interface{}
	i := 1 // Argument index counter

	// Basic table name sanitization
	safeTable := req.Table
	if strings.ContainsAny(safeTable, ";'\" ") {
		log.Printf("Error: Invalid table name used in buildUpdateQuery: %s", safeTable)
		return "", nil
	}

	// Build the SET clause
	for col, val := range req.Values {
		// Basic column name sanitization
		if !strings.ContainsAny(col, ";'\" ") {
			// Special handling for boolean roles remains
			if col == "R1" || col == "R2" || col == "R3" || col == "R4" {
				// Explicitly convert string "true"/"false" to boolean for DB
				boolVal := strings.ToLower(fmt.Sprintf("%v", val)) == "true"
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
				args = append(args, boolVal) // Append actual boolean
			} else {
				// For other columns (like password_hash), use the value directly
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
				args = append(args, val) // Append value as-is (string hash, etc.)
			}
			i++ // Increment argument index
		} else {
			log.Printf("Warning: Invalid column name skipped in UPDATE SET clause: %s", col)
		}
	}

	if len(setClauses) == 0 {
		log.Printf("Error: No valid columns found for UPDATE SET clause for table %s", safeTable)
		return "", nil
	}

	// Build the WHERE clause, continuing index from SET
	whereClause, whereArgs := buildWhereClause(req.Where, i) // Pass current index 'i'
	if whereClause == "" {                                   // Require WHERE for safety
		log.Printf("Error: WHERE clause is required for UPDATE on table %s", safeTable)
		return "", nil
	}

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", safeTable, strings.Join(setClauses, ", "), whereClause)
	args = append(args, whereArgs...) // Append WHERE clause arguments

	return query, args
}

// buildDeleteQuery constructs a DELETE SQL query string and its arguments.
func buildDeleteQuery(req QueryRequest) (string, []interface{}) {
	// Basic table name sanitization
	safeTable := req.Table
	if strings.ContainsAny(safeTable, ";'\" ") {
		log.Printf("Error: Invalid table name used in buildDeleteQuery: %s", safeTable)
		return "", nil
	}

	query := fmt.Sprintf("DELETE FROM %s", safeTable)
	var args []interface{}

	if len(req.Where) > 0 {
		whereClause, whereArgs := buildWhereClause(req.Where)
		if whereClause != "" {
			query += " WHERE " + whereClause
			args = whereArgs
		} else {
			// If where clause was provided but invalid, prevent accidental full delete
			log.Printf("Error: Invalid WHERE clause provided for DELETE on table %s", safeTable)
			return "", nil
		}
	} else if !req.DeleteAll {
		// If no WHERE and DeleteAll is false, prevent delete
		log.Printf("Error: WHERE clause or 'delete_all' flag required for DELETE on table %s", safeTable)
		return "", nil
	}
	// If DeleteAll is true and no WHERE, the query remains "DELETE FROM table"

	return query, args
}

// buildWhereClause builds the WHERE part of a query.
// startIndex is optional; if provided, parameter placeholders ($1, $2) start from this index.
func buildWhereClause(where map[string]string, startIndex ...int) (string, []interface{}) {
	var clauses []string
	var args []interface{}

	// Determine the starting index; default to 1 if not provided
	i := 1
	if len(startIndex) > 0 && startIndex[0] > 0 {
		i = startIndex[0]
	}

	// Build WHERE clause
	for col, val := range where {
		// Basic column name sanitization
		if !strings.ContainsAny(col, ";'\" ") {
			clauses = append(clauses, fmt.Sprintf("%s = $%d", col, i))
			args = append(args, val) // Value is used as parameter, safe from injection here
			i++
		} else {
			log.Printf("Warning: Invalid column name skipped in WHERE clause: %s", col)
		}
	}

	if len(clauses) == 0 {
		return "", nil // Return empty if no valid clauses
	}

	return strings.Join(clauses, " AND "), args
}
