package api

import (
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"sync"
)

type catalogCaptureKey struct{}
type catalogCaptureBuffer struct {
	mu       sync.Mutex
	entities []catalog.CapturedEntity
	size     int
	overflow bool
}

func (b *catalogCaptureBuffer) add(entities []catalog.CapturedEntity) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.overflow {
		return
	}
	for _, entity := range entities {
		b.size += len(entity.Payload)
		for _, relation := range entity.Relations {
			b.size += len(relation.Metadata)
		}
		if b.size > 8<<20 || len(b.entities) >= 20000 {
			b.overflow = true
			b.entities = nil
			return
		}
		b.entities = append(b.entities, entity)
	}
}

func (b *catalogCaptureBuffer) snapshot() ([]catalog.CapturedEntity, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.overflow {
		return nil, false
	}
	unique := []catalog.CapturedEntity{}
	seen := map[string]int{}
	for _, entity := range b.entities {
		key := entity.EntityType + ":" + entity.ID + ":" + entity.Resource
		if index, ok := seen[key]; ok {
			if len(entity.Payload) > len(unique[index].Payload) {
				unique[index] = entity
			}
		} else {
			seen[key] = len(unique)
			unique = append(unique, entity)
		}
	}
	return unique, true
}
