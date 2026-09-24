// Command canonicalwav creates benchmark-ready mono PCM16 WAV files by using
// MediaHub's production WAV, MP3, and Ogg/Vorbis decoders.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
)

func main() {
	input := flag.String("input", "", "source WAV, MP3, or Ogg/Vorbis file")
	output := flag.String("output", "", "destination canonical PCM16 WAV")
	flag.Parse()
	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "canonicalwav: -input and -output are required")
		os.Exit(2)
	}
	result, err := analysis.WriteCanonicalWAV(context.Background(), analysis.NewDefaultDecoderRegistry(), *input, *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canonicalwav: %v\n", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "canonicalwav: encode metadata: %v\n", err)
		os.Exit(1)
	}
}
