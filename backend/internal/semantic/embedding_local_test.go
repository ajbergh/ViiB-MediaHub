package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagedModelCacheVerifiesAndWorksOffline(t *testing.T) {
	content := []byte("pretrained model fixture")
	hash := sha256.Sum256(content)
	artifact := modelArtifact{"weights", int64(len(content)), hex.EncodeToString(hash[:])}
	dir := t.TempDir()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/weights" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected outbound request: %s %s", r.Method, r.URL)
		}
		_, _ = w.Write(content)
	}))
	if err := ensureModelArtifact(context.Background(), server.Client(), server.URL+"/", dir, artifact); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := ensureModelArtifact(context.Background(), server.Client(), server.URL+"/", dir, artifact); err != nil {
		t.Fatalf("offline cache: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests=%d", requests)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact.name), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureModelArtifact(context.Background(), server.Client(), server.URL+"/", dir, artifact); err == nil {
		t.Fatal("corrupt cache accepted offline")
	}
}

func TestManagedModelRejectsBadOrPartialDownloads(t *testing.T) {
	good := []byte("good")
	hash := sha256.Sum256(good)
	artifact := modelArtifact{"weights", 4, hex.EncodeToString(hash[:])}
	for _, body := range []string{"bad!", "go", "good-extra"} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			if err := ensureModelArtifact(context.Background(), server.Client(), server.URL+"/", dir, artifact); err == nil {
				t.Fatal("invalid download accepted")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatalf("partial files remain: %v", entries)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ensureModelArtifact(ctx, http.DefaultClient, "https://example.invalid/", t.TempDir(), artifact); err == nil {
		t.Fatal("canceled download succeeded")
	}
}

func TestLocalWordPieceAndNormalizedPooling(t *testing.T) {
	provider := &localEmbeddingProvider{vocab: map[string]int{"cafe": 0, "music": 1, "##al": 2, "!": 3, "中": 4, "文": 5}, vectors: make([]float32, 6*LocalEmbeddingDimensions)}
	provider.vectors[0] = 1
	provider.vectors[LocalEmbeddingDimensions+1] = 1
	provider.vectors[2*LocalEmbeddingDimensions+2] = 1
	ids := provider.tokenize("Café MUSICAL! 中文 missing")
	if !reflect.DeepEqual(ids, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("tokens=%v", ids)
	}
	vector, err := provider.EmbedQuery(context.Background(), "Café musical")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if math.Abs(float64(vector[i])-1/math.Sqrt(3)) > 1e-6 {
			t.Fatalf("pooling=%v", vector[:3])
		}
	}
	if _, err := provider.EmbedQuery(context.Background(), "unrecognized"); err == nil {
		t.Fatal("empty token vector accepted")
	}
}

func TestLocalTensorLoaderRejectsInvalidShapeAndBounds(t *testing.T) {
	for _, shape := range [][]int{{2, 256}, {1, 128}} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "vocab.txt"), []byte("music\n"), 0600); err != nil {
			t.Fatal(err)
		}
		header, _ := json.Marshal(map[string]any{"embeddings": map[string]any{"dtype": "F32", "shape": shape, "data_offsets": []int{0, 1024}}})
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, uint64(len(header)))
		data = append(data, header...)
		if err := os.WriteFile(filepath.Join(dir, "model.safetensors"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadLocalEmbeddingProvider(dir); err == nil {
			t.Fatal("malformed tensor accepted")
		}
	}
}

// Explicitly opt in: downloads ~31 MB only when VIIB_LOCAL_MODEL_TEST_DIR is set.
// CI's ordinary unit tests never require network access or model weights.
func TestLocalModelReferenceAndMusicRelevance(t *testing.T) {
	dir := os.Getenv("VIIB_LOCAL_MODEL_TEST_DIR")
	if dir == "" {
		t.Skip("set VIIB_LOCAL_MODEL_TEST_DIR to run real-model validation")
	}
	provider, err := NewLocalEmbeddingProvider(context.Background(), dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	fixtureBytes, err := os.ReadFile("testdata/local_embedding_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Text   string
		Tokens []int
		Vector []float32
	}
	if err := json.Unmarshal(fixtureBytes, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		local := provider.(*localEmbeddingProvider)
		if ids := local.tokenize(fixture.Text); !reflect.DeepEqual(ids, fixture.Tokens) {
			t.Fatalf("tokenizer mismatch %q: %v vs %v", fixture.Text, ids, fixture.Tokens)
		}
		vector, err := provider.EmbedQuery(context.Background(), fixture.Text)
		if err != nil {
			t.Fatal(err)
		}
		for i, expected := range fixture.Vector {
			if math.Abs(float64(vector[i]-expected)) > 1e-5 {
				t.Fatalf("reference mismatch %q dimension %d: %f vs %f", fixture.Text, i, vector[i], expected)
			}
		}
	}
	for _, pair := range [][3]string{
		{"peaceful relaxing piano music", "gentle acoustic piano instrumental for calm relaxation", "aggressive heavy metal guitar screaming vocals"},
		{"energetic dance music", "upbeat electronic dance techno house club", "quiet classical chamber music string quartet"},
	} {
		vectors, err := provider.EmbedDocuments(context.Background(), pair[:])
		if err != nil {
			t.Fatal(err)
		}
		if cosineSimilarity(vectors[0], vectors[1]) <= cosineSimilarity(vectors[0], vectors[2]) {
			t.Fatalf("relevance ordering failed: %s", strings.Join(pair[:], " / "))
		}
	}
}
