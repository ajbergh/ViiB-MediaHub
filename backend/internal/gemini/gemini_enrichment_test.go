package gemini

import "testing"

func TestParseTOONLineExcludesBPMAndKey(t *testing.T) {
	id, metadata, err := parseTOONLine("song|House;Dance|energetic|high|fast|false|1999")
	if err != nil {
		t.Fatal(err)
	}
	if id != "song" || metadata.Tempo != "fast" || metadata.OriginalYear != 1999 {
		t.Fatalf("parsed enrichment = id %q metadata %#v", id, metadata)
	}
	if _, _, err := parseTOONLine("song|House|energetic|high|fast|128|false|1999"); err == nil {
		t.Fatal("legacy enrichment response containing BPM was accepted")
	}
}
