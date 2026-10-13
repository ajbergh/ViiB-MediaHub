package track

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"io"
	"testing"
)

func TestPreparationValidatesCorruptPayloadBeforeSkippingCurrentScalars(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	artifact, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Data = []byte("corrupt nonempty gzip")
	if err := database.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	opens := 0
	p, err := Run(t.Context(), database, registry, ids, RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		s, e := analysis.ResolveLocalSource(database, id)
		open := s.Open
		s.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
		return s, e
	}})
	if err != nil || p.Analyzed != 1 || opens != 1 {
		t.Fatalf("corrupt current artifact skipped: %+v %v opens=%d", p, err, opens)
	}
	repaired, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := features.DecodeBounded(repaired.Data, features.MaxStructureStatusArtifactBytes); err != nil {
		t.Fatal("corrupt feature not repaired")
	}
}
