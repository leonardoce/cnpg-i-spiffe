// Package agent contains the implementation of the sidecar agent: it
// fetches and rotates X.509 SVIDs from the SPIRE Workload API, writes them
// into the shared certs volume, and reloads PostgreSQL's configuration
// after every rotation.
package agent
