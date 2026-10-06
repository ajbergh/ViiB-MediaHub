package api

import (
	"github.com/ajbergh/viib-mediahub/internal/db"
	"testing"
)

func TestLocalScalarAlternativesRemainSeparateInResponse(t *testing.T) {
	localBPM, providerBPM, manualBPM := 120.0, 130.0, 140.0
	tonic, providerTonic, mode, spotify := 0, 9, "minor", "spotify"
	a := db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisComplete, SourceFingerprint: "bytes1", BPM: &providerBPM, BPMSource: &spotify, KeyTonic: &providerTonic, KeyMode: &mode, KeySource: &spotify,
		Local: &db.LocalScalarObservation{SourceFingerprint: "bytes1", AlgorithmVersion: "local-v1", BPM: &localBPM, KeyTonic: &tonic, KeyMode: &mode}}
	a.SpotifyBindings = &db.SpotifyScalarBindings{BPM: &db.SpotifyScalarBinding{SourceFingerprint: "bytes1", Durable: true, Eligible: true}, Key: &db.SpotifyScalarBinding{SourceFingerprint: "bytes1", Durable: true, Eligible: true}}
	r := trackAnalysisFeatureResponseWithCurrentSource(a, db.TrackAnalysisOverride{}, "bytes1")
	if r.BPMSource != spotify || *r.BPM != providerBPM || *r.MeasuredBPM != localBPM || *r.MeasuredKeyTonic != tonic || *r.KeyTonic != providerTonic {
		t.Fatalf("alternatives: %+v", r)
	}
	o := db.TrackAnalysisOverride{BPM: &manualBPM, BPMLocked: true, BPMSourceFingerprint: "bytes1", KeyTonic: &tonic, KeyMode: &mode, KeyLocked: true, KeySourceFingerprint: "bytes1"}
	r = trackAnalysisFeatureResponseWithCurrentSource(a, o, "bytes1")
	if r.BPMSource != "manual" || *r.BPM != manualBPM || r.KeySource != "manual" || *r.MeasuredBPM != localBPM {
		t.Fatal("manual override destroyed alternatives")
	}
	r = trackAnalysisFeatureResponseWithCurrentSource(a, o, "bytes2")
	if r.MeasuredBPM != nil || r.MeasuredKeyTonic != nil || r.LocalAlgorithmVersion != nil {
		t.Fatal("stale local observation exposed")
	}
}
