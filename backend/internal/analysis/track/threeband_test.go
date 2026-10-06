package track

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"io"
	"reflect"
	"testing"
)

func TestThreeBandRepairPreservesOtherPreparation(t *testing.T) {
	d, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), d, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	before, _ := d.GetTrackAnalysis(ids[0])
	wave, _ := d.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	cues, _ := d.GetDJHotCues(ids[0])
	band, err := d.GetTrackAnalysisArtifact(ids[0], threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if band.SourceFingerprint != before.SourceFingerprint {
		t.Fatal("band unbound")
	}
	if _, err := threeband.Decode(band.Data); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteTrackAnalysisArtifact(ids[0], threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); err != nil {
		t.Fatal(err)
	}
	selected, err := d.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 1 {
		t.Fatal("missing bands not selected")
	}
	opens := 0
	opts := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		s, e := analysis.ResolveLocalSource(d, id)
		open := s.Open
		s.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
		return s, e
	}}
	progress, err := Run(t.Context(), d, registry, selected, opts)
	if err != nil || progress.Analyzed != 1 || opens != 1 {
		t.Fatalf("repair %v %v opens %d", progress, err, opens)
	}
	after, _ := d.GetTrackAnalysis(ids[0])
	retained, _ := d.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	afterCues, _ := d.GetDJHotCues(ids[0])
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(wave, retained) || !reflect.DeepEqual(cues, afterCues) {
		t.Fatal("unrelated preparation changed")
	}
	selected, err = d.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 0 {
		t.Fatal("repaired band selected again")
	}
}
