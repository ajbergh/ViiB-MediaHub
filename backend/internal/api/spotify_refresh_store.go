package api

import (
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"time"
)

// A service captures this owner once. Reads and writes validate ownership
// transactionally; cooldowns include only its captured private context.
type spotifyRuntimeRefreshStore struct {
	*db.DB
	fence db.SpotifyMetadataFence
}

var _ refresh.Store = spotifyRuntimeRefreshStore{}

func (s spotifyRuntimeRefreshStore) PutExternalAnalysis(o analysis.Observation, revision string, expires time.Time) error {
	return s.DB.PutExternalAnalysisForRuntime(s.fence, o, revision, expires)
}

func (s spotifyRuntimeRefreshStore) PutExternalAnalysisStatus(id, endpoint string, status db.ExternalAnalysisStatus) error {
	return s.DB.PutExternalAnalysisStatusForRuntime(s.fence, id, endpoint, status)
}

func (s spotifyRuntimeRefreshStore) GetExternalAnalysis(id, endpoint string) (*db.ExternalAnalysisCache, error) {
	return s.DB.GetExternalAnalysisForRuntime(db.SpotifyMetadataReadFence{Epoch: s.fence.Epoch, ContextKey: s.fence.ContextKey}, id, endpoint)
}
func (s spotifyRuntimeRefreshStore) GetExternalAnalysisStatus(id, endpoint string) (*db.ExternalAnalysisStatus, error) {
	return s.DB.GetExternalAnalysisStatusForRuntime(db.SpotifyMetadataReadFence{Epoch: s.fence.Epoch, ContextKey: s.fence.ContextKey}, id, endpoint)
}
func (s spotifyRuntimeRefreshStore) GetExternalAnalysisCooldown() (time.Time, error) {
	return s.DB.GetExternalAnalysisCooldownForRuntime(s.fence)
}
