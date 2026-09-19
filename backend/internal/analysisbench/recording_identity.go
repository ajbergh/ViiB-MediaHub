package analysisbench

import (
	"sort"
	"strings"
)

// RecordingIdentityAudit does not infer acoustic identity from different file
// hashes. It exposes declared recording groups and blocks counting multiple
// encodings of one recording as independent accuracy observations.
type RecordingIdentityAudit struct {
	UnidentifiedTracks int                      `json:"unidentifiedTracks"`
	DistinctGroups     int                      `json:"distinctGroups"`
	CrossSplitGroups   int                      `json:"crossSplitGroups"`
	HeldOutOnlyGroups  int                      `json:"heldOutOnlyGroups"`
	RepeatedGroups     []RepeatedRecordingGroup `json:"repeatedGroups,omitempty"`
}

type RepeatedRecordingGroup struct {
	Group    string   `json:"group"`
	TrackIDs []string `json:"trackIds"`
	Splits   []string `json:"splits"`
}

func AuditRecordingIdentity(manifest CorpusManifest) RecordingIdentityAudit {
	var audit RecordingIdentityAudit
	groups := make(map[string][]CorpusTrack)
	for _, track := range manifest.Tracks {
		group := strings.TrimSpace(track.RecordingGroup)
		if group == "" {
			audit.UnidentifiedTracks++
			continue
		}
		groups[group] = append(groups[group], track)
	}
	audit.DistinctGroups = len(groups)
	for group, tracks := range groups {
		heldOutOnly := true
		for _, track := range tracks {
			heldOutOnly = heldOutOnly && track.Split == SplitHeldOut
		}
		if heldOutOnly {
			audit.HeldOutOnlyGroups++
		}
		if len(tracks) < 2 {
			continue
		}
		repeated := RepeatedRecordingGroup{Group: group}
		splits := make(map[string]bool)
		for _, track := range tracks {
			repeated.TrackIDs = append(repeated.TrackIDs, track.ID)
			splits[track.Split] = true
		}
		for split := range splits {
			repeated.Splits = append(repeated.Splits, split)
		}
		sort.Strings(repeated.TrackIDs)
		sort.Strings(repeated.Splits)
		if len(splits) > 1 {
			audit.CrossSplitGroups++
		}
		audit.RepeatedGroups = append(audit.RepeatedGroups, repeated)
	}
	sort.Slice(audit.RepeatedGroups, func(i, j int) bool { return audit.RepeatedGroups[i].Group < audit.RepeatedGroups[j].Group })
	return audit
}
