package analysisbench

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteCorpusManifest writes a validated manifest without overwriting prior
// evidence. Importers and fixture generators share this invariant.
func WriteCorpusManifest(path string, manifest CorpusManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create corpus manifest %q: %w", path, err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(manifest)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write corpus manifest %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close corpus manifest %q: %w", path, closeErr)
	}
	return nil
}
