// Package metadata bounds and sanitizes provider domain snapshots before storage.
package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const CatalogLimit = 2 << 20
const ScalarLimit = 64 << 10
const DetailedLimit = 8 << 20

// Sanitize preserves nesting, nulls, zero, false and unknown domain fields.
// Sensitive fields are removed recursively. HTTP envelopes are never accepted here.
func Sanitize(raw []byte, limit int) ([]byte, error) {
	if limit <= 0 || limit > DetailedLimit || len(raw) > limit {
		return nil, errors.New("metadata payload exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid metadata JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("invalid metadata JSON trailing data")
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, errors.New("metadata must be a domain object")
	}
	for _, name := range []string{"headers", "request", "response", "cookies", "authorization"} {
		if _, ok := object[name]; ok {
			return nil, errors.New("metadata cannot contain transport envelope")
		}
	}
	if err := clean(value, 0); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > limit {
		return nil, errors.New("metadata payload exceeds limit")
	}
	return encoded, nil
}

func sensitive(name string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(name))
	return strings.Contains(normalized, "token") || strings.Contains(normalized, "cookie") || strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || normalized == "authorization" || normalized == "headers" || normalized == "credentials"
}
func clean(value any, depth int) error {
	if depth > 64 {
		return errors.New("metadata nesting exceeds limit")
	}
	switch node := value.(type) {
	case map[string]any:
		for name, child := range node {
			if sensitive(name) {
				delete(node, name)
				continue
			}
			if err := clean(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := clean(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
