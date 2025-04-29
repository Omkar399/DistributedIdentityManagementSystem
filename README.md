# Distributed Identity Management System
An Identity management system used to track IDs region-wise. The system allows admins to create/assign, update, and delete roles/access to users across a distributed network of nodes.

## System Features
- **User Management**: Create, update, and delete user identities across distributed nodes
- **Real-time Monitoring**: View replication status, operation history, and node health
- **Node Control**: Start/stop nodes to simulate network failures and recovery
- **Transaction Logs**: Inspect operation logs showing system activity

## System Design
### Backend Architecture
+ **Membership List**: Tracks which processes receive multicast messages
  + Runs as an individual service where all nodes send heartbeats to the server
  + Each node has a key in the server with a lease and expiry time
    + Lease is automatically renewed with successive heartbeats
    + Node is removed if no heartbeat is received before lease expiration

+ **Leader Election**: 
  + Uses Quorum-based voting
  + Implements a Raft-like consensus algorithm to identify the leader

+ **Multicast Spanning Tree**: Updates from the leader database are propagated to replica nodes
  + Algorithm ensures the leader is always the root of the tree
  + Tree is balanced using AVL tree algorithm for consistent construction across nodes
  + Optimizes network traffic during state updates

+ **Consistency & Fault Tolerance**: 
  + New or recovered nodes sync with the current leader by requesting transaction logs
  + Log-based recovery mechanism restores consistent state after failures
  + System automatically handles node failures with data resynchronization

### Frontend Components
+ **Node Control Panel**: Manage and monitor distributed nodes
+ **Replication Status**: View real-time consistency state across nodes
+ **Operations Queue**: Track operation propagation with status indicators
+ **User Management**: Interface for identity CRUD operations
+ **Log Viewer**: Transaction log inspection tool

## Deployment
### Backend
```bash
# Start the backend services
cd backend
docker compose up --build
```

### Frontend
```bash
# Install dependencies
cd frontend
npm install

# Run development server
npm run serve

# Build for production
npm run build
```

## Accessing the Application
Once both backend and frontend are running:
- Frontend UI: http://localhost:8080
- API Endpoints: http://localhost:8090

## System Requirements
- Docker and Docker Compose
- Node.js v14+
- Go 1.19+
