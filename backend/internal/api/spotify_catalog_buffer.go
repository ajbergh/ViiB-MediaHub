package api

import (
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
	"strings"
	"sync"
	"unicode"
)

type catalogCaptureKey struct{}
type catalogCaptureBuffer struct {
	mu              sync.Mutex
	playlistID      string
	revision        string
	revisionInvalid bool
	entities        []catalog.CapturedEntity
	size            int
	overflow        bool
}

func (b *catalogCaptureBuffer) add(entities []catalog.CapturedEntity) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.overflow {
		return
	}
	for _, entity := range entities {
		if entity.EntityType == "playlist" && entity.ID == b.playlistID && b.revision != "" {
			if entity.CaptureRevision != "" && entity.CaptureRevision != b.revision {
				b.revisionInvalid = true
				return
			}
			entity.CaptureRevision = b.revision
		}
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
	if b.overflow || b.revisionInvalid {
		return nil, false
	}

	return catalog.DeduplicateCapturedEntities(b.entities), true

}

// Root capture occurs before its compatibility response is decoded. Bind it
// after the caller accepts the revision, then tag later pages in the same buffer.
func (b *catalogCaptureBuffer) bindPlaylistRevision(id, revision string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.revisionInvalid || revision == "" || len(revision) > 256 || strings.IndexFunc(revision, unicode.IsControl) >= 0 || (b.playlistID != "" && (b.playlistID != id || b.revision != revision)) {
		return false
	}
	for i := range b.entities {
		entity := &b.entities[i]
		if entity.EntityType != "playlist" || entity.ID != id {
			continue
		}
		if entity.CaptureRevision != "" && entity.CaptureRevision != revision {
			b.revisionInvalid = true
			return false
		}
		entity.CaptureRevision = revision
	}
	b.playlistID = id
	b.revision = revision
	return true
}
