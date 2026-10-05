// Defines database lock functionality for package db.

package db

import (
	"fmt"
	"os"
)

// Shared application locks allow multiple readers; restore requires exclusive
// ownership. OS locks are released after crashes without stale PID cleanup.
func lockDatabase(path string, exclusive bool) (*os.File, error) {
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockDatabaseFile(file, exclusive); err != nil {
		file.Close()
		return nil, fmt.Errorf("database is in use; close ViiB before restore: %w", err)
	}
	return file, nil
}
