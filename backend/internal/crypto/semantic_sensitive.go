package crypto

// Semantic embedding providers can use a cloud API key. Registering the
// setting explicitly keeps it inside the existing machine-bound encryption
// mechanism; key suffixes are not treated as sensitive automatically.
func init() {
	sensitiveKeys["semantic_embedding_api_key"] = true
}
