// Command viib-restore applies a validated restore staged by ViiB MediaHub.
// Run it only after the main application has fully exited.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func main() {
	dataDir := flag.String("data", "", "ViiB MediaHub data directory")
	flag.Parse()
	if *dataDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
		*dataDir = filepath.Join(configDir, "ViiB-MediaHub")
	}
	applied, rollback, err := db.ApplyPendingRestore(*dataDir)
	if err != nil { fmt.Fprintf(os.Stderr, "restore failed: %v\n", err); os.Exit(1) }
	if !applied { fmt.Println("No staged restore was found."); return }
	fmt.Println("Restore applied successfully.")
	if rollback != "" { fmt.Printf("Rollback database: %s\n", rollback) }
}
