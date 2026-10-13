package analysis

import (
	"encoding/json"
	"reflect"
)

// sanitizeScalars isolates invalid optional measurements before strict normalization.
// Only known field paths and stable reasons are retained, never rejected values.
func sanitizeScalars(body []byte, features bool) ([]byte, []FieldRejection, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	fields := envelope
	prefix := ""
	if !features {
		fields = nil
		if err := json.Unmarshal(envelope["track"], &fields); err != nil {
			return nil, nil, err
		}
		prefix = "track."
	}
	var rejected []FieldRejection
	typ := reflect.TypeOf(track{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := field.Tag.Get("json")
		raw, exists := fields[name]
		if !exists {
			continue
		}
		target := reflect.New(field.Type)
		reason := ""
		if err := json.Unmarshal(raw, target.Interface()); err != nil {
			reason = "invalid_type"
		} else if !target.Elem().IsNil() {
			value := target.Elem().Elem()
			if value.Kind() == reflect.Int {
				n := value.Int()
				if (name == "key" && (n < -1 || n > 11)) || (name == "mode" && (n < 0 || n > 1)) || (name == "time_signature" && n <= 0) {
					reason = "out_of_range"
				}
			} else {
				n := value.Float()
				if !finite(&n) {
					reason = "non_finite"
				} else if name == "tempo" || name == "duration" {
					if n <= 0 {
						reason = "out_of_range"
					}
				} else if name != "loudness" && !confidence(&n) {
					reason = "out_of_range"
				}
			}
		}
		if reason != "" {
			delete(fields, name)
			rejected = append(rejected, FieldRejection{prefix + name, reason})
		}
	}
	if features {
		if raw, ok := fields["duration_ms"]; ok {
			var duration *float64
			reason := ""
			if json.Unmarshal(raw, &duration) != nil {
				reason = "invalid_type"
			} else if !finite(duration) || (duration != nil && *duration <= 0) {
				reason = "out_of_range"
			}
			if reason != "" {
				delete(fields, "duration_ms")
				rejected = append(rejected, FieldRejection{"duration_ms", reason})
			}
		}
	} else {
		encoded, err := json.Marshal(fields)
		if err != nil {
			return nil, nil, err
		}
		envelope["track"] = encoded
	}
	encoded, err := json.Marshal(envelope)
	return encoded, rejected, err
}
