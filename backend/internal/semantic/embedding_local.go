package semantic

// ViiB's managed Model2Vec runtime: WordPiece tokenization followed by mean
// pooling of pretrained token vectors and L2 normalization. No external process,
// API credentials, native libraries, or chat-provider settings are involved.
import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	EmbeddingProviderLocal = "local"
	localModelRevision     = "bf8b056651a2c21b8d2565580b8569da283cab23"
	// Include inference/preprocessing version: changing either requires new vectors.
	LocalEmbeddingModel      = "minishlab/potion-base-8M@" + localModelRevision + "/viib-v1"
	LocalEmbeddingDimensions = 256
	localModelURL            = "https://huggingface.co/minishlab/potion-base-8M/resolve/" + localModelRevision + "/"
)

//go:embed model2vec-license.txt
var model2vecLicense []byte

type modelArtifact struct {
	name   string
	size   int64
	sha256 string
}

var localModelArtifacts = []modelArtifact{
	{"vocab.txt", 219690, "1394523a67ddd404a825428018c0582a6998bcfa044ecbcbf1f4d71adb94c61c"},
	{"model.safetensors", 30236760, "f65d0f325faadc1e121c319e2faa41170d3fa07d8c89abd48ca5358d9a223de2"},
}

func ManagedEmbeddingResolution() EmbeddingResolution {
	return EmbeddingResolution{Settings: EmbeddingSettings{Provider: EmbeddingProviderLocal, Model: LocalEmbeddingModel, Dimensions: LocalEmbeddingDimensions}, Status: embeddingResolutionReady, Source: "managed_local"}
}

// NewLocalEmbeddingProvider acquires public model files only. Library content is
// never sent over the network. Validated cached files support offline startup.
func NewLocalEmbeddingProvider(ctx context.Context, dataDir string, client *http.Client, progress func(float64)) (EmbeddingProvider, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("local model cache directory is missing")
	}
	dir := filepath.Join(dataDir, "models", "potion-base-8M-"+localModelRevision)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	for i, artifact := range localModelArtifacts {
		if err := ensureModelArtifact(ctx, client, localModelURL, dir, artifact); err != nil {
			return nil, fmt.Errorf("prepare local Smart Playlist model: %w", err)
		}
		if progress != nil {
			progress(float64(i+1) / float64(len(localModelArtifacts)))
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "LICENSE.txt"), model2vecLicense, 0600); err != nil {
		return nil, err
	}
	return loadLocalEmbeddingProvider(dir)
}

func ensureModelArtifact(ctx context.Context, client *http.Client, baseURL, dir string, artifact modelArtifact) error {
	path := filepath.Join(dir, artifact.name)
	if validateModelArtifact(path, artifact) == nil {
		return ctx.Err()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+artifact.name, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("model download returned HTTP %d", response.StatusCode)
	}
	file, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, artifact.size+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != artifact.size || hex.EncodeToString(hash.Sum(nil)) != artifact.sha256 {
		return fmt.Errorf("model file %s failed integrity verification", artifact.name)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Windows cannot atomically replace an existing file; only remove a corrupt
	// cache entry after its verified replacement is fully written and closed.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(file.Name(), path)
}

func validateModelArtifact(path string, artifact modelArtifact) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, artifact.size+1))
	if err != nil {
		return err
	}
	if n != artifact.size || hex.EncodeToString(hash.Sum(nil)) != artifact.sha256 {
		return fmt.Errorf("invalid model cache file %s", artifact.name)
	}
	return nil
}

type localEmbeddingProvider struct {
	vocab   map[string]int
	vectors []float32
}

func loadLocalEmbeddingProvider(dir string) (*localEmbeddingProvider, error) {
	vocabBytes, err := os.ReadFile(filepath.Join(dir, "vocab.txt"))
	if err != nil {
		return nil, err
	}
	tokens := strings.Split(strings.TrimSuffix(string(vocabBytes), "\n"), "\n")
	vocab := make(map[string]int, len(tokens))
	for i, token := range tokens {
		vocab[strings.TrimSuffix(token, "\r")] = i
	}
	data, err := os.ReadFile(filepath.Join(dir, "model.safetensors"))
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("invalid safetensors header")
	}
	headerSize := binary.LittleEndian.Uint64(data[:8])
	if headerSize > uint64(len(data)-8) {
		return nil, fmt.Errorf("invalid safetensors header length")
	}
	var tensors map[string]struct {
		Dtype   string `json:"dtype"`
		Shape   []int  `json:"shape"`
		Offsets []int  `json:"data_offsets"`
	}
	if err := json.Unmarshal(data[8:8+headerSize], &tensors); err != nil {
		return nil, err
	}
	tensor, ok := tensors["embeddings"]
	if !ok || tensor.Dtype != "F32" || len(tensor.Shape) != 2 || tensor.Shape[0] != len(tokens) || tensor.Shape[1] != LocalEmbeddingDimensions || len(tensor.Offsets) != 2 {
		return nil, fmt.Errorf("unexpected local embedding tensor")
	}
	payload := data[8+headerSize:]
	start, end := tensor.Offsets[0], tensor.Offsets[1]
	if start < 0 || end < start || end > len(payload) || end-start != len(tokens)*LocalEmbeddingDimensions*4 {
		return nil, fmt.Errorf("invalid local embedding tensor bounds")
	}
	vectors := make([]float32, len(tokens)*LocalEmbeddingDimensions)
	for i := range vectors {
		vectors[i] = math.Float32frombits(binary.LittleEndian.Uint32(payload[start+i*4:]))
		if math.IsNaN(float64(vectors[i])) || math.IsInf(float64(vectors[i]), 0) {
			return nil, fmt.Errorf("non-finite model weight")
		}
	}
	return &localEmbeddingProvider{vocab: vocab, vectors: vectors}, nil
}

func (*localEmbeddingProvider) Name() string           { return EmbeddingProviderLocal }
func (*localEmbeddingProvider) Model() string          { return LocalEmbeddingModel }
func (*localEmbeddingProvider) Dimensions() int        { return LocalEmbeddingDimensions }
func (*localEmbeddingProvider) DocumentPrefix() string { return "" }
func (*localEmbeddingProvider) QueryPrefix() string    { return "" }
func (*localEmbeddingProvider) MaxBatchSize() int      { return 128 }
func (*localEmbeddingProvider) Close() error           { return nil }

func (provider *localEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vector, err := provider.EmbedQuery(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors[i] = vector
	}
	return vectors, nil
}

func (provider *localEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vector := make([]float32, LocalEmbeddingDimensions)
	ids := provider.tokenize(text)
	if len(ids) == 0 {
		return nil, fmt.Errorf("text contains no known embedding tokens")
	}
	for _, id := range ids {
		for i, value := range provider.vectors[id*LocalEmbeddingDimensions : (id+1)*LocalEmbeddingDimensions] {
			vector[i] += value
		}
	}
	return NormalizeL2(vector) // Mean pooling and sum pooling have identical L2-normalized output.
}

// Implements the pinned model's lowercase BertNormalizer/BertPreTokenizer and
// greedy WordPiece rules. No CLS/SEP tokens; unknown words are omitted, matching
// Model2Vec StaticModel.tokenize. All input tokens participate in pooling.
func (provider *localEmbeddingProvider) tokenize(text string) []int {
	var ids []int
	for len(text) > 0 {
		position, special := len(text), ""
		for _, token := range []string{"[PAD]", "[UNK]", "[CLS]", "[SEP]", "[MASK]"} {
			if i := strings.Index(text, token); i >= 0 && i < position {
				position, special = i, token
			}
		}
		ids = append(ids, provider.tokenizeRegular(text[:position])...)
		if special == "" {
			break
		}
		if id, ok := provider.vocab[special]; ok && special != "[UNK]" {
			ids = append(ids, id)
		}
		text = text[position+len(special):]
	}
	return ids
}

func (provider *localEmbeddingProvider) tokenizeRegular(text string) []int {
	var cleaned strings.Builder
	for _, r := range text {
		if r == 0 || r == unicode.ReplacementChar || (unicode.In(r, unicode.Cc, unicode.Cf) && r != '\t' && r != '\n' && r != '\r') {
			continue
		}
		if unicode.IsSpace(r) {
			cleaned.WriteRune(' ')
			continue
		}
		if isBertChinese(r) {
			cleaned.WriteRune(' ')
			cleaned.WriteRune(r)
			cleaned.WriteRune(' ')
			continue
		}
		cleaned.WriteRune(r)
	}
	var normalized strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(cleaned.String())) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsPunct(r) || (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
			normalized.WriteRune(' ')
			normalized.WriteRune(r)
			normalized.WriteRune(' ')
		} else {
			normalized.WriteRune(r)
		}
	}
	var ids []int
	for _, word := range strings.Fields(normalized.String()) {
		runes := []rune(word)
		if len(runes) > 100 {
			continue
		}
		var pieces []int
		for start := 0; start < len(runes); {
			found := false
			for end := len(runes); end > start; end-- {
				piece := string(runes[start:end])
				if start > 0 {
					piece = "##" + piece
				}
				if id, ok := provider.vocab[piece]; ok {
					pieces = append(pieces, id)
					start = end
					found = true
					break
				}
			}
			if !found {
				pieces = nil
				break
			}
		}
		ids = append(ids, pieces...)
	}
	return ids
}

func isBertChinese(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x20000 && r <= 0x2A6DF) || (r >= 0x2A700 && r <= 0x2B73F) || (r >= 0x2B740 && r <= 0x2B81F) || (r >= 0x2B820 && r <= 0x2CEAF) || (r >= 0xF900 && r <= 0xFAFF) || (r >= 0x2F800 && r <= 0x2FA1F)
}
