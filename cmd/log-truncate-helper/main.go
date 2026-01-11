// log-truncate-helper is a setuid helper for truncating Docker container logs.
// This is required because the main dashboard runs with NoNewPrivileges=true.
//
// Install with: sudo install -o root -g docker -m 4750 log-truncate-helper /usr/local/bin/
//
// Usage: log-truncate-helper <container-log-path>
//
// Security:
// - Only accepts paths under /var/lib/docker/containers/
// - Only truncates files ending in .log
// - Runs as root but only callable by docker group members
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	dockerContainersDir = "/var/lib/docker/containers/"
	logSuffix           = ".log"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <container-log-path>\n", os.Args[0])
		os.Exit(1)
	}

	logPath := os.Args[1]

	// Validate the path
	if err := validatePath(logPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Truncate the file
	if err := os.Truncate(logPath, 0); err != nil {
		fmt.Fprintf(os.Stderr, "Error truncating file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully truncated %s\n", logPath)
}

// validatePath ensures the path is a valid Docker container log file.
func validatePath(path string) error {
	// Clean and resolve the path
	cleanPath := filepath.Clean(path)

	// Must be under Docker containers directory
	if !strings.HasPrefix(cleanPath, dockerContainersDir) {
		return fmt.Errorf("path must be under %s", dockerContainersDir)
	}

	// Must end with .log
	if !strings.HasSuffix(cleanPath, logSuffix) {
		return fmt.Errorf("path must end with %s", logSuffix)
	}

	// Check that the file exists and is a regular file
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}

	// Additional check: path should not contain ".." after cleaning
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path contains invalid components")
	}

	return nil
}
