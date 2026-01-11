package services

import (
	"os"
	"os/exec"
	"path/filepath"
)

// findExecutable searches for an executable in common paths
// This is needed because packaged macOS apps don't have access to user's PATH
func findExecutable(name string) string {
	homeDir, _ := os.UserHomeDir()

	// Common paths to check
	searchPaths := []string{
		"/opt/homebrew/bin",                                         // Homebrew (Apple Silicon)
		"/usr/local/bin",                                            // Homebrew (Intel) / system
		"/usr/bin",                                                  // System
		"/opt/anaconda3/bin",                                        // Anaconda Python
		"/opt/miniconda3/bin",                                       // Miniconda Python
		filepath.Join(homeDir, "anaconda3", "bin"),                  // User Anaconda
		filepath.Join(homeDir, "miniconda3", "bin"),                 // User Miniconda
		filepath.Join(homeDir, ".local", "bin"),                     // pip user install
		filepath.Join(homeDir, "Library", "Python", "3.13", "bin"),  // pip Python 3.13
		filepath.Join(homeDir, "Library", "Python", "3.12", "bin"),  // pip Python 3.12
		filepath.Join(homeDir, "Library", "Python", "3.11", "bin"),  // pip Python 3.11
		filepath.Join(homeDir, "Library", "Python", "3.10", "bin"),  // pip Python 3.10
	}

	// First try exec.LookPath (works when running from terminal)
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	// Check common paths
	for _, dir := range searchPaths {
		fullPath := filepath.Join(dir, name)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	// Return original name as fallback
	return name
}
