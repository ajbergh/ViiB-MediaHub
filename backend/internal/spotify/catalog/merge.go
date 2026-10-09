package catalog

// DeduplicateCapturedEntities combines repeated projections of the same
// entity/resource without discarding relation coverage gathered from another
// occurrence. Conflicting identities at one relation position are represented
// as unavailable rather than choosing a possibly incorrect edge.
func DeduplicateCapturedEntities(entities []CapturedEntity) []CapturedEntity {
	type entityKey struct{ entityType, id, resource string }
	type relationKey struct {
		kind     string
		position int
	}
	unique := make([]CapturedEntity, 0, len(entities))
	indices := make(map[entityKey]int, len(entities))
	conflicts := make(map[int]map[relationKey]bool)
	for _, entity := range entities {
		key := entityKey{entity.EntityType, entity.ID, entity.Resource}
		index, exists := indices[key]
		if !exists {
			indices[key] = len(unique)
			unique = append(unique, cloneCapturedEntity(entity))
			continue
		}
		current := &unique[index]
		if len(entity.Payload) > len(current.Payload) {
			current.Payload = append([]byte(nil), entity.Payload...)
		}
		if current.CaptureRevision == "" {
			current.CaptureRevision = entity.CaptureRevision
		}
		positions := make(map[relationKey]int, len(current.Relations))
		for i, relation := range current.Relations {
			positions[relationKey{relation.Kind, relation.Position}] = i
		}
		for _, candidate := range entity.Relations {
			relationID := relationKey{candidate.Kind, candidate.Position}
			currentIndex, found := positions[relationID]
			if !found {
				positions[relationID] = len(current.Relations)
				current.Relations = append(current.Relations, cloneCapturedRelation(candidate))
				continue
			}
			if conflicts[index][relationID] {
				continue
			}
			prior := current.Relations[currentIndex]
			priorValid := capturedRelationValid(prior)
			candidateValid := capturedRelationValid(candidate)
			switch {
			case priorValid && candidateValid && (prior.ChildType != candidate.ChildType || prior.ChildID != candidate.ChildID):
				prior.ChildType, prior.ChildID, prior.Unavailable = "", "", true
				prior.Metadata = longerBytes(prior.Metadata, candidate.Metadata)
				if conflicts[index] == nil {
					conflicts[index] = make(map[relationKey]bool)
				}
				conflicts[index][relationID] = true
			case priorValid && candidateValid:
				prior.Metadata = longerBytes(prior.Metadata, candidate.Metadata)
			case !priorValid && candidateValid:
				prior = cloneCapturedRelation(candidate)
			case !priorValid && !candidateValid:
				prior.ChildType, prior.ChildID, prior.Unavailable = "", "", true
				prior.Metadata = longerBytes(prior.Metadata, candidate.Metadata)
			}
			current.Relations[currentIndex] = prior
		}
	}
	return unique
}

func cloneCapturedEntity(entity CapturedEntity) CapturedEntity {
	entity.Payload = append([]byte(nil), entity.Payload...)
	relations := entity.Relations
	entity.Relations = make([]CapturedRelation, len(relations))
	for i, relation := range relations {
		entity.Relations[i] = cloneCapturedRelation(relation)
	}
	return entity
}

func cloneCapturedRelation(relation CapturedRelation) CapturedRelation {
	relation.Metadata = append([]byte(nil), relation.Metadata...)
	return relation
}

func capturedRelationValid(relation CapturedRelation) bool {
	return !relation.Unavailable && relation.ChildType != "" && relation.ChildID != ""
}

func longerBytes(a, b []byte) []byte {
	if len(b) > len(a) {
		return append([]byte(nil), b...)
	}
	return append([]byte(nil), a...)
}
