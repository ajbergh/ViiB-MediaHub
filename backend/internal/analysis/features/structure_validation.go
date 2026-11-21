// Reads bounded structure artifacts and caches validation facts for identical payloads.
package features

import (
	"bytes"
	"compress/gzip"
	"container/list"
	"crypto/sha256"
	"errors"
	"io"
	"math"
	"sync"
)

type structureInflater struct {
	input bytes.Reader
	gzip  gzip.Reader
}

var structureInflaters = sync.Pool{New: func() any { return new(structureInflater) }}

func readBoundedStructure(data []byte, maxBytes int64) ([]byte, error) {
	inflater := structureInflaters.Get().(*structureInflater)
	inflater.input.Reset(data)
	defer func() {
		inflater.input.Reset(nil)
		structureInflaters.Put(inflater)
	}()
	if err := inflater.gzip.Reset(&inflater.input); err != nil {
		return nil, err
	}
	decoded, readErr := io.ReadAll(io.LimitReader(&inflater.gzip, maxBytes+1))
	closeErr := inflater.gzip.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(decoded)) > maxBytes {
		return nil, errors.New("energy artifact exceeds decoded size limit")
	}
	return decoded, nil
}

// StructureValidation describes reusable facts about one exact encoded payload.
// Callers still check current source identity and artifact provenance.
type StructureValidation struct {
	Decodable     bool
	Native        bool
	ValidSections bool
}

type structureValidationKey struct {
	digest [sha256.Size]byte
	limit  int64
}

type structureValidationEntry struct {
	key    structureValidationKey
	result StructureValidation
}

const structureValidationCacheLimit = 65_536

var structureValidationCache = struct {
	sync.Mutex
	entries map[structureValidationKey]*list.Element
	order   list.List
}{entries: make(map[structureValidationKey]*list.Element)}

// InspectStructureBounded reuses validation of identical bytes in this process.
// The cache holds flags only and distinguishing byte limits preserves fail-closed reads.
func InspectStructureBounded(data []byte, maxBytes int64) StructureValidation {
	key := structureValidationKey{digest: sha256.Sum256(data), limit: maxBytes}
	cache := &structureValidationCache
	cache.Lock()
	if hit := cache.entries[key]; hit != nil {
		cache.order.MoveToFront(hit)
		result := hit.Value.(structureValidationEntry).result
		cache.Unlock()
		return result
	}
	cache.Unlock()

	var validation StructureValidation
	if result, err := DecodeBounded(data, maxBytes); err == nil {
		validation.Decodable = true
		validation.Native = result.ValidateNativeEvents() == nil
		validation.ValidSections = validStructureSections(result.Sections)
	}

	cache.Lock()
	defer cache.Unlock()
	if hit := cache.entries[key]; hit != nil {
		cache.order.MoveToFront(hit)
		return hit.Value.(structureValidationEntry).result
	}
	if cache.order.Len() >= structureValidationCacheLimit {
		oldest := cache.order.Back()
		delete(cache.entries, oldest.Value.(structureValidationEntry).key)
		cache.order.Remove(oldest)
	}
	cache.entries[key] = cache.order.PushFront(structureValidationEntry{key: key, result: validation})
	return validation
}

func validStructureSections(sections []Section) bool {
	if len(sections) == 0 {
		return false
	}
	for _, section := range sections {
		if math.IsNaN(section.Start) || math.IsInf(section.Start, 0) || section.Start < 0 ||
			math.IsNaN(section.End) || math.IsInf(section.End, 0) || section.End <= section.Start ||
			math.IsNaN(section.Energy) || section.Energy < 0 || section.Energy > 1 ||
			math.IsNaN(section.Confidence) || section.Confidence < 0 || section.Confidence > 1 {
			return false
		}
		switch section.Label {
		case StructureIntro, StructureBuild, StructureDrop, StructureBreakdown, StructureOutro, StructureUnknown:
		default:
			return false
		}
	}
	return true
}
