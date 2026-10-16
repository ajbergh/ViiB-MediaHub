package analysis

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func avBytes(tag protowire.Number, value []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, tag, protowire.BytesType), value)
}
func avInt(tag protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, tag, protowire.VarintType), value)
}
func avDouble(value float64) []byte {
	return protowire.AppendFixed64(protowire.AppendTag(nil, 1, protowire.Fixed64Type), math.Float64bits(value))
}
func avDomain(bpm float64, tonic string, mode uint64, camelot string) []byte {
	key := append(avBytes(1, []byte(tonic)), avInt(2, mode)...)
	key = append(key, avBytes(3, avBytes(1, []byte(camelot)))...)
	return append(avDouble(bpm), avBytes(2, key)...)
}
func avResponse(id, typ string, domain []byte, status uint64) []byte {
	any := append(avBytes(1, []byte(typ)), avBytes(2, domain)...)
	entity := append(avBytes(1, avInt(1, status)), avBytes(2, []byte("spotify:track:"+id))...)
	entity = append(entity, avBytes(3, any)...)
	ext := append(avInt(2, attributesExtension), avBytes(3, entity)...)
	return avBytes(2, ext)
}

func TestAudioAttributesV2Contract(t *testing.T) {
	at := time.Now()
	for _, tc := range []struct {
		mode    uint64
		want    int
		camelot string
	}{{2, 1, "5B"}, {1, 0, "2A"}} {
		raw := avResponse(testID, attributesTypeURL, avDomain(70.4827, "D#", tc.mode, tc.camelot), 200)
		o, err := decodeAttributesResponse(raw, testID, at)
		if err != nil {
			t.Fatal(err)
		}
		if o.BPM == nil || *o.BPM != 70.4827 || o.Key == nil || *o.Key != 3 || o.Mode == nil || *o.Mode != tc.want || o.Camelot == nil || *o.Camelot != tc.camelot || o.SourceEndpoint != AttributesV2 || o.RetrievedAt != at.UTC() {
			t.Fatalf("wrong V2 observation: %+v", o)
		}
		if o.BPMConfidence != nil || o.KeyConfidence != nil || o.DurationSeconds != nil || o.Energy != nil || len(o.DomainPayload) != 0 {
			t.Fatal("invented unsupported V2 data")
		}
		if err := ValidateObservation(o); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAudioAttributesV2RejectsBadEnvelopeAndDomain(t *testing.T) {
	valid := avResponse(testID, attributesTypeURL, avDomain(70.4827, "D#", 2, "5B"), 200)
	for _, raw := range [][]byte{
		{0xff}, valid[:len(valid)-1], append(append([]byte(nil), valid...), valid...),
		avResponse("wrong", attributesTypeURL, avDomain(70.4827, "D#", 2, "5B"), 200),
		avResponse(testID, "type.googleapis.com/other", avDomain(70.4827, "D#", 2, "5B"), 200),
		avResponse(testID, attributesTypeURL, avDomain(math.NaN(), "D#", 2, "5B"), 200),
		avResponse(testID, attributesTypeURL, avDomain(0, "D#", 2, "5B"), 200),
		avResponse(testID, attributesTypeURL, avDomain(70, "D#", 3, "5B"), 200),
		avResponse(testID, attributesTypeURL, avDomain(70, "D#", 2, "4B"), 200),
	} {
		_, err := decodeAttributesResponse(raw, testID, time.Now())
		assertCode(t, err, ProviderChanged)
	}
	_, err := decodeAttributesResponse(avResponse(testID, attributesTypeURL, nil, 404), testID, time.Now())
	assertCode(t, err, NotFound)
	o, err := decodeAttributesResponse(avResponse(testID, attributesTypeURL, avDouble(70.4827), 200), testID, time.Now())
	if err != nil || o.Key != nil || o.Mode != nil || o.Camelot != nil {
		t.Fatalf("missing key invented: %+v %v", o, err)
	}
}

func TestAudioAttributesV2HTTPRefreshAndRequest(t *testing.T) {
	var calls, refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/extended-metadata/v0/extended-metadata" || r.Header.Get("Accept") != "application/protobuf" || r.Header.Get("Spotify-App-Version") != "fixture" {
			t.Errorf("wrong request: %s %s", r.Method, r.URL)
		}
		var request struct {
			Entities []struct {
				URI     string `json:"entityUri"`
				Queries []struct {
					Kind int `json:"extensionKind"`
				} `json:"query"`
			} `json:"entityRequest"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Entities) != 1 || request.Entities[0].URI != "spotify:track:"+testID || len(request.Entities[0].Queries) != 1 || request.Entities[0].Queries[0].Kind != 222 {
			t.Error("wrong extension request")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "Bearer new" {
			t.Error("did not use renewed token")
		}
		w.Write(avResponse(testID, attributesTypeURL, avDomain(70.4827, "D#", 2, "5B"), 200))
	}))
	defer server.Close()
	client := NewClient(testTokens(&refreshes), Options{Enabled: true, AppVersion: "fixture", AccountContext: "account", Client: testHTTP(server)})
	o, err := client.FetchAudioAttributesV2(context.Background(), testID)
	if err != nil || refreshes.Load() != 1 || calls.Load() != 2 || o.AccountContext != "account" {
		t.Fatalf("V2 refresh: %+v %v", o, err)
	}
}

func TestV2PreparationPreservesMetadataAndOrigins(t *testing.T) {
	bpm, energy := 137.98, .4
	key, mode := 8, 1
	legacy := &Observation{TrackID: testID, AccountContext: "account", SourceEndpoint: "audio_features", BPM: &bpm, Key: &key, Mode: &mode, Energy: &energy, RetrievedAt: time.Now()}
	v2, err := decodeAttributesDomain(avDomain(70.4827, "D#", 2, "5B"), testID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	v2.AccountContext = "account"
	got := PreferAudioAttributesV2(legacy, &v2)
	if *got.BPM != 70.4827 || *got.Key != 3 || got.Energy != legacy.Energy || got.BPMOrigin != &v2 || got.KeyOrigin != &v2 || *legacy.BPM != 137.98 {
		t.Fatalf("merge lost V2 or metadata: %+v", got)
	}
	partial := v2
	partial.BPM = nil
	if got := PreferAudioAttributesV2(legacy, &partial); *got.BPM != 137.98 || *got.Key != 3 {
		t.Fatal("partial V2 discarded independent legacy field")
	}
	v2.AccountContext = "other"
	if got := PreferAudioAttributesV2(legacy, &v2); *got.BPM != 137.98 {
		t.Fatal("cross-account merge")
	}
}
