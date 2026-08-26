package crypto

import "testing"

func TestSemanticEmbeddingAPIKeyIsSensitive(t *testing.T) {
	if !IsSensitiveKey("semantic_embedding_api_key") {
		t.Fatal("semantic embedding API key must use the encrypted setting mechanism")
	}
}
