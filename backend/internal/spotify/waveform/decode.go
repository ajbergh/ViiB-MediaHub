// Package waveform decodes the pinned Web Player extension 237 contract.
package waveform

import (
	"errors"
	"math"
	"regexp"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	ExtensionKind    = 237
	TypeURL          = "type.googleapis.com/spotify.playlistmixing.extensions.mixthreebandwaveforms.ThreeBandWaveforms"
	ContractRevision = "web-player.a6d2a638-extension237-v1"
	MaxBody          = 8 << 20
	MaxSamples       = 1000000
)

var validID = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)
var ErrMalformed = errors.New("Spotify waveform malformed payload")

type EntityError struct{ Status int }

func (e *EntityError) Error() string { return "Spotify waveform entity unavailable" }

type Waveform struct {
	TrackID            string  `json:"trackId,omitempty"`
	SampleRate         int32   `json:"sampleRate"`
	WindowMilliseconds int32   `json:"windowMilliseconds"`
	Lows               []int32 `json:"lows"`
	Mids               []int32 `json:"mids"`
	Highs              []int32 `json:"highs"`
	ETag               string  `json:"etag,omitempty"`
	Domain             []byte  `json:"-"`
}

func (w Waveform) DurationSeconds() float64 {
	return float64(len(w.Lows)) * float64(w.WindowMilliseconds) / 1000
}
func (w Waveform) Validate() error {
	if w.SampleRate < 8000 || w.SampleRate > 384000 || w.WindowMilliseconds <= 0 || w.WindowMilliseconds > 10000 || len(w.Lows) == 0 || len(w.Lows) > MaxSamples || len(w.Mids) != len(w.Lows) || len(w.Highs) != len(w.Lows) || len(w.ETag) > 1024 {
		return ErrMalformed
	}
	return nil
}

// Aligned uses the existing recording-match tolerance, plus one sample window
// for quantized overview duration. It never changes actual playback timing.
func (w Waveform) Aligned(duration float64) bool {
	if w.Validate() != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return false
	}
	tolerance := math.Min(3, math.Max(2, duration*.005)) + float64(w.WindowMilliseconds)/1000
	return math.Abs(w.DurationSeconds()-duration) <= tolerance
}

type wireField struct {
	kind   protowire.Type
	number uint64
	data   []byte
}

func fields(raw []byte) (map[protowire.Number][]wireField, error) {
	if len(raw) > MaxBody {
		return nil, ErrMalformed
	}
	result := map[protowire.Number][]wireField{}
	for len(raw) > 0 {
		tag, kind, n := protowire.ConsumeTag(raw)
		if n < 0 || tag <= 0 {
			return nil, ErrMalformed
		}
		raw = raw[n:]
		field := wireField{kind: kind}
		switch kind {
		case protowire.VarintType:
			field.number, n = protowire.ConsumeVarint(raw)
		case protowire.BytesType:
			field.data, n = protowire.ConsumeBytes(raw)
		default:
			n = protowire.ConsumeFieldValue(tag, kind, raw)
		}
		if n < 0 {
			return nil, ErrMalformed
		}
		raw = raw[n:]
		// Unknown fields are consumed but excluded from the typed envelope.
		if tag <= 5 {
			if len(result[tag]) >= 64 {
				return nil, ErrMalformed
			}
			result[tag] = append(result[tag], field)
		}
	}
	return result, nil
}
func one(values map[protowire.Number][]wireField, tag protowire.Number, kind protowire.Type, required bool) (wireField, error) {
	entries := values[tag]
	if len(entries) == 0 && !required {
		return wireField{kind: kind}, nil
	}
	if len(entries) != 1 || entries[0].kind != kind {
		return wireField{}, ErrMalformed
	}
	return entries[0], nil
}
func DecodeDomain(raw []byte) (Waveform, error) {
	if len(raw) > MaxBody {
		return Waveform{}, ErrMalformed
	}
	original := raw
	w := Waveform{}
	rateSeen, windowSeen := false, false
	bands := []*[]int32{&w.Lows, &w.Mids, &w.Highs}
	appendValue := func(band *[]int32, n uint64) error {
		if len(*band) >= MaxSamples || (n > math.MaxUint32 && n < 0xffffffff80000000) {
			return ErrMalformed
		}
		*band = append(*band, int32(n))
		return nil
	}
	for len(raw) > 0 {
		tag, kind, n := protowire.ConsumeTag(raw)
		if n < 0 || tag <= 0 {
			return Waveform{}, ErrMalformed
		}
		raw = raw[n:]
		if tag == 1 || tag == 2 {
			if kind != protowire.VarintType {
				return Waveform{}, ErrMalformed
			}
			value, count := protowire.ConsumeVarint(raw)
			if count < 0 || value > math.MaxInt32 {
				return Waveform{}, ErrMalformed
			}
			raw = raw[count:]
			if tag == 1 {
				if rateSeen {
					return Waveform{}, ErrMalformed
				}
				rateSeen = true
				w.SampleRate = int32(value)
			} else {
				if windowSeen {
					return Waveform{}, ErrMalformed
				}
				windowSeen = true
				w.WindowMilliseconds = int32(value)
			}
		} else if tag >= 3 && tag <= 5 {
			band := bands[int(tag)-3]
			switch kind {
			case protowire.VarintType:
				value, count := protowire.ConsumeVarint(raw)
				if count < 0 {
					return Waveform{}, ErrMalformed
				}
				raw = raw[count:]
				if err := appendValue(band, value); err != nil {
					return Waveform{}, err
				}
			case protowire.BytesType:
				data, count := protowire.ConsumeBytes(raw)
				if count < 0 {
					return Waveform{}, ErrMalformed
				}
				raw = raw[count:]
				for len(data) > 0 {
					value, count := protowire.ConsumeVarint(data)
					if count < 0 {
						return Waveform{}, ErrMalformed
					}
					data = data[count:]
					if err := appendValue(band, value); err != nil {
						return Waveform{}, err
					}
				}
			default:
				return Waveform{}, ErrMalformed
			}
		} else {
			count := protowire.ConsumeFieldValue(tag, kind, raw)
			if count < 0 {
				return Waveform{}, ErrMalformed
			}
			raw = raw[count:]
		}
	}
	if err := w.Validate(); err != nil {
		return Waveform{}, err
	}
	w.Domain = append([]byte(nil), original...)
	return w, nil
}

func DecodeResponse(raw []byte, id string) (Waveform, error) {
	if !validID.MatchString(id) || len(raw) > MaxBody {
		return Waveform{}, ErrMalformed
	}
	root, err := fields(raw)
	if err != nil {
		return Waveform{}, err
	}
	if len(root[2]) > 32 {
		return Waveform{}, ErrMalformed
	}
	var selected *Waveform
	for _, extension := range root[2] {
		if extension.kind != protowire.BytesType {
			return Waveform{}, ErrMalformed
		}
		object, err := fields(extension.data)
		if err != nil {
			return Waveform{}, err
		}
		kind, err := one(object, 2, protowire.VarintType, true)
		if err != nil {
			return Waveform{}, err
		}
		if kind.number != ExtensionKind {
			continue
		}
		header, err := one(object, 1, protowire.BytesType, false)
		if err != nil {
			return Waveform{}, err
		}
		provider, err := fields(header.data)
		if err != nil {
			return Waveform{}, err
		}
		status, err := one(provider, 1, protowire.VarintType, false)
		if err != nil {
			return Waveform{}, err
		}
		if status.number > 599 {
			return Waveform{}, ErrMalformed
		}
		if status.number != 0 && status.number != 200 {
			return Waveform{}, &EntityError{Status: int(status.number)}
		}
		if len(object[3]) > 32 {
			return Waveform{}, ErrMalformed
		}
		for _, entity := range object[3] {
			if entity.kind != protowire.BytesType {
				return Waveform{}, ErrMalformed
			}
			value, err := fields(entity.data)
			if err != nil {
				return Waveform{}, err
			}
			uri, err := one(value, 2, protowire.BytesType, true)
			if err != nil || string(uri.data) != "spotify:track:"+id {
				return Waveform{}, ErrMalformed
			}
			header, err := one(value, 1, protowire.BytesType, true)
			if err != nil {
				return Waveform{}, err
			}
			entityHeader, err := fields(header.data)
			if err != nil {
				return Waveform{}, err
			}
			status, err := one(entityHeader, 1, protowire.VarintType, true)
			if err != nil {
				return Waveform{}, err
			}
			if status.number < 100 || status.number > 599 {
				return Waveform{}, ErrMalformed
			}
			if status.number != 200 {
				return Waveform{}, &EntityError{Status: int(status.number)}
			}
			etag, err := one(entityHeader, 2, protowire.BytesType, false)
			if err != nil || len(etag.data) > 1024 {
				return Waveform{}, ErrMalformed
			}
			anyField, err := one(value, 3, protowire.BytesType, true)
			if err != nil {
				return Waveform{}, err
			}
			anyObject, err := fields(anyField.data)
			if err != nil {
				return Waveform{}, err
			}
			typ, err := one(anyObject, 1, protowire.BytesType, true)
			if err != nil || string(typ.data) != TypeURL {
				return Waveform{}, ErrMalformed
			}
			domain, err := one(anyObject, 2, protowire.BytesType, true)
			if err != nil {
				return Waveform{}, err
			}
			decoded, err := DecodeDomain(domain.data)
			if err != nil {
				return Waveform{}, err
			}
			decoded.ETag = string(etag.data)
			decoded.TrackID = id
			if selected != nil {
				return Waveform{}, ErrMalformed
			}
			selected = &decoded
		}
	}
	if selected == nil {
		return Waveform{}, ErrMalformed
	}
	return *selected, nil
}
