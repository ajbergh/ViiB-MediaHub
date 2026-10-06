package analysis

import "encoding/json"

// Invalid detailed siblings are omitted independently, without discarding scalars
// or other interval capabilities. Full valid objects retain their unknown fields.
func sanitizeDetailedArrays(body []byte) ([]byte, []FieldRejection, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, err
	}
	var t track
	if err := json.Unmarshal(root["track"], &t); err != nil {
		return nil, nil, err
	}
	var rejected []FieldRejection
	for _, name := range []string{"bars", "beats", "tatums", "sections", "segments"} {
		raw, exists := root[name]
		if !exists {
			continue
		}
		valid := true
		if name == "segments" {
			var values []segment
			if json.Unmarshal(raw, &values) != nil || len(values) > 40000 {
				valid = false
			} else {
				for _, value := range values {
					if !validInterval(value.interval, t.Duration) {
						valid = false
						break
					}
					for _, vector := range [][]float64{value.Pitches, value.Timbre} {
						if vector != nil && len(vector) != 12 {
							valid = false
							break
						}
						for _, n := range vector {
							if !finite(&n) {
								valid = false
								break
							}
						}
					}
				}
			}
		} else {
			var values []interval
			if json.Unmarshal(raw, &values) != nil || len(values) > 20000 {
				valid = false
			} else {
				for _, value := range values {
					if !validInterval(value, t.Duration) {
						valid = false
						break
					}
				}
			}
		}
		// Validate known optional detailed measurements without reducing the stored object.
		var objects []map[string]json.RawMessage
		if json.Unmarshal(raw, &objects) != nil {
			valid = false
		}
		for _, object := range objects {
			for _, field := range []string{"key", "mode", "time_signature"} {
				if rawValue, ok := object[field]; ok {
					var n *int
					if json.Unmarshal(rawValue, &n) != nil {
						valid = false
						continue
					}
					if n != nil && ((field == "key" && (*n < -1 || *n > 11)) || (field == "mode" && (*n < 0 || *n > 1)) || (field == "time_signature" && *n <= 0)) {
						valid = false
					}
				}
			}
			var duration, maxTime *float64
			_ = json.Unmarshal(object["duration"], &duration)
			_ = json.Unmarshal(object["loudness_max_time"], &maxTime)
			if duration != nil && maxTime != nil && *maxTime > *duration {
				valid = false
			}
			for _, field := range []string{"loudness_start", "loudness_max", "loudness_end", "loudness_max_time", "loudness", "tempo", "tempo_confidence", "key_confidence", "mode_confidence", "time_signature_confidence"} {
				if rawValue, ok := object[field]; ok {
					var n *float64
					if json.Unmarshal(rawValue, &n) != nil || !finite(n) {
						valid = false
						continue
					}
					if n != nil && ((field == "loudness_max_time" && *n < 0) || (field == "tempo" && *n <= 0) || (len(field) >= 11 && field[len(field)-11:] == "_confidence" && !confidence(n))) {
						valid = false
					}
				}
			}
		}
		if !valid {
			delete(root, name)
			rejected = append(rejected, FieldRejection{Path: name, Reason: "invalid_artifact"})
		}
	}
	encoded, err := json.Marshal(root)
	return encoded, rejected, err
}
