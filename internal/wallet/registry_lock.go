package wallet

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// registryLock manages a simple file-based lock for the wallet registry
type registryLock struct {
	lockPath string
	locked   bool
}

// newRegistryLock creates a lock for the wallet registry
func newRegistryLock() *registryLock {
	stateDir, _ := os.UserHomeDir()
	return &registryLock{
		lockPath: filepath.Join(stateDir, ".mozartpay", "state", "wallets.lock"),
	}
}

// Lock attempts to acquire the registry lock with a timeout
func (l *registryLock) Lock() error {
	if l.locked {
		return fmt.Errorf("already locked")
	}

	// Ensure state directory exists
	lockDir := filepath.Dir(l.lockPath)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Try to create lock file exclusively (fails if it exists)
	// This is a simple approach - for production, consider using github.com/gofrs/flock
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for registry lock")
		case <-ticker.C:
			// Try to create the lock file
			file, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				// Write PID to lock file for debugging
				fmt.Fprintf(file, "%d", os.Getpid())
				file.Close()
				l.locked = true
				return nil
			}
			// Lock file exists, retry
		}
	}
}

// Unlock releases the registry lock
func (l *registryLock) Unlock() error {
	if !l.locked {
		return nil
	}
	l.locked = false
	return os.Remove(l.lockPath)
}
