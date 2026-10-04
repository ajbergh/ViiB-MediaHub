// Command spotifyreferenceexport freezes normalized cached references without
// provider access, database mutation, or inferred recording confirmation.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func main() {
	databasePath := flag.String("database", "", "existing library SQLite path, opened read-only")
	manifestPath := flag.String("manifest", "", "identity-bound corpus manifest; paths resolve against current directory")
	outputPath := flag.String("out", "", "new frozen snapshot JSON path; never overwritten")
	endpoint := flag.String("endpoint", "", "explicit cached endpoint: audio_features or audio_analysis")
	license := flag.String("reference-license", "", "reference retention/license declaration")
	labelSource := flag.String("reference-label-source", "", "reference label provenance declaration")
	flag.Parse()
	if *databasePath == "" || *manifestPath == "" || *outputPath == "" || *endpoint == "" || *license == "" || *labelSource == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "spotifyreferenceexport: database, manifest, out, endpoint, reference-license and reference-label-source are required")
		os.Exit(2)
	}
	if err := run(*databasePath, *manifestPath, *outputPath, analysisbench.SpotifyReferenceExportOptions{Endpoint: *endpoint, License: *license, LabelSource: *labelSource, Now: time.Now().UTC()}); err != nil {
		fmt.Fprintf(os.Stderr, "spotifyreferenceexport: %v\n", err)
		os.Exit(1)
	}
}

func run(databasePath, manifestPath, outputPath string, options analysisbench.SpotifyReferenceExportOptions) error {
	manifest, err := analysisbench.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	database, err := db.OpenReferenceReadOnly(databasePath)
	if err != nil {
		return fmt.Errorf("open read-only reference database: %w", err)
	}
	defer database.Close()
	interrupted, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(interrupted, 10*time.Minute)
	defer cancel()
	snapshot, coverage, err := analysisbench.ExportSpotifyReference(ctx, database, manifest, options)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(coverage); err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("no eligible cached references; no snapshot written")
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create new snapshot: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(outputPath) // Only the file this invocation exclusively created.
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return nil
}
