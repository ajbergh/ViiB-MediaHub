package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"google.golang.org/protobuf/encoding/protowire"
)

const AttributesV2 = "audio_attributes_v2"
const AttributesV2Revision = "web-player-extension222-v1"
const attributesExtension = 222
const attributesTypeURL = "type.googleapis.com/spotify.playlistmixing.extensions.audio_attributes.v2.AudioAttributes"

// FetchAudioAttributesV2 requests the recording-scoped metadata used by Web
// Player mixing. It supplies tempo/key only, never beat phase or confidence.
func (c *Client) FetchAudioAttributesV2(ctx context.Context, id string) (Observation, error) {
	if !c.enabled || c.tokens == nil || c.appVersion == "" || strings.ContainsAny(c.appVersion, "\r\n") {
		return Observation{}, failure(Disabled, 0)
	}
	if !trackID.MatchString(id) {
		return Observation{}, failure(InvalidTrackID, 0)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	token, err := c.tokens.Token(ctx, auth.InternalAnalysis)
	if err != nil {
		return Observation{}, tokenError(ctx, err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if token.Kind != auth.WebPlayer || token.Bearer() == "" {
			return Observation{}, failure(AuthenticationRequired, 0)
		}
		body, _ := json.Marshal(map[string]any{"entityRequest": []any{map[string]any{"entityUri": "spotify:track:" + id, "query": []any{map[string]any{"extensionKind": attributesExtension, "etag": ""}}}}})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://spclient.wg.spotify.com/extended-metadata/v0/extended-metadata", bytes.NewReader(body))
		if err != nil {
			return Observation{}, failure(ProviderChanged, 0)
		}
		req.Header.Set("Authorization", "Bearer "+token.Bearer())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/protobuf")
		req.Header.Set("App-Platform", "WebPlayer")
		req.Header.Set("Spotify-App-Version", c.appVersion)
		response, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return Observation{}, ctx.Err()
			}
			return Observation{}, failure(TemporarilyUnavailable, 0)
		}
		if response.StatusCode == 401 && attempt == 0 {
			response.Body.Close()
			token, err = c.tokens.Refresh(ctx, auth.InternalAnalysis, token)
			if err != nil {
				return Observation{}, tokenError(ctx, err)
			}
			continue
		}
		if response.StatusCode != 200 {
			_, err = c.decode(response, id)
			response.Body.Close()
			return Observation{}, err
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		if ctx.Err() != nil {
			return Observation{}, ctx.Err()
		}
		if err != nil {
			return Observation{}, failure(TemporarilyUnavailable, 200)
		}
		if len(raw) > 1<<20 {
			return Observation{}, failure(ProviderChanged, 200)
		}
		o, err := decodeAttributesResponse(raw, id, c.now())
		o.AccountContext = c.accountContext
		return o, err
	}
	return Observation{}, failure(AuthenticationRequired, 401)
}

type attributeField struct {
	kind   protowire.Type
	number uint64
	data   []byte
}
type attributeFields map[protowire.Number][]attributeField

func readAttributeFields(raw []byte) (attributeFields, error) {
	if len(raw) > 1<<20 {
		return nil, failure(ProviderChanged, 200)
	}
	result := attributeFields{}
	for count := 0; len(raw) > 0; count++ {
		if count >= 256 {
			return nil, failure(ProviderChanged, 200)
		}
		tag, kind, n := protowire.ConsumeTag(raw)
		if n < 0 || tag <= 0 {
			return nil, failure(ProviderChanged, 200)
		}
		raw = raw[n:]
		f := attributeField{kind: kind}
		switch kind {
		case protowire.BytesType:
			f.data, n = protowire.ConsumeBytes(raw)
		case protowire.VarintType:
			f.number, n = protowire.ConsumeVarint(raw)
		case protowire.Fixed64Type:
			f.number, n = protowire.ConsumeFixed64(raw)
		default:
			n = protowire.ConsumeFieldValue(tag, kind, raw)
		}
		if n < 0 {
			return nil, failure(ProviderChanged, 200)
		}
		raw = raw[n:]
		result[tag] = append(result[tag], f)
	}
	return result, nil
}

func (f attributeFields) one(tag protowire.Number, kind protowire.Type, required bool) (attributeField, error) {
	v := f[tag]
	if len(v) == 0 && !required {
		return attributeField{kind: kind}, nil
	}
	if len(v) != 1 || v[0].kind != kind {
		return attributeField{}, failure(ProviderChanged, 200)
	}
	return v[0], nil
}

func attributeStatus(raw []byte, required bool) error {
	f, err := readAttributeFields(raw)
	if err != nil {
		return err
	}
	s, err := f.one(1, protowire.VarintType, required)
	if err != nil {
		return err
	}
	if s.number == 0 && !required || s.number == 200 {
		return nil
	}
	code := ProviderChanged
	switch s.number {
	case 404:
		code = NotFound
	case 403, 451:
		code = AccessDenied
	case 429:
		code = RateLimited
	case 204:
		code = AnalysisUnavailable
	case 500, 502, 503:
		code = TemporarilyUnavailable
	}
	return failure(code, int(s.number))
}

func decodeAttributesResponse(raw []byte, id string, now time.Time) (Observation, error) {
	root, err := readAttributeFields(raw)
	if err != nil {
		return Observation{}, err
	}
	h, err := root.one(1, protowire.BytesType, false)
	if err != nil {
		return Observation{}, err
	}
	if err = attributeStatus(h.data, false); err != nil {
		return Observation{}, err
	}
	var selected *Observation
	for _, ext := range root[2] {
		if ext.kind != protowire.BytesType {
			return Observation{}, failure(ProviderChanged, 200)
		}
		f, err := readAttributeFields(ext.data)
		if err != nil {
			return Observation{}, err
		}
		kind, err := f.one(2, protowire.VarintType, true)
		if err != nil {
			return Observation{}, err
		}
		if kind.number != attributesExtension {
			continue
		}
		h, err := f.one(1, protowire.BytesType, false)
		if err != nil {
			return Observation{}, err
		}
		if err = attributeStatus(h.data, false); err != nil {
			return Observation{}, err
		}
		for _, entity := range f[3] {
			if entity.kind != protowire.BytesType {
				return Observation{}, failure(ProviderChanged, 200)
			}
			e, err := readAttributeFields(entity.data)
			if err != nil {
				return Observation{}, err
			}
			uri, err := e.one(2, protowire.BytesType, true)
			if err != nil || string(uri.data) != "spotify:track:"+id {
				return Observation{}, failure(ProviderChanged, 200)
			}
			h, err := e.one(1, protowire.BytesType, true)
			if err != nil {
				return Observation{}, err
			}
			if err = attributeStatus(h.data, true); err != nil {
				return Observation{}, err
			}
			a, err := e.one(3, protowire.BytesType, true)
			if err != nil {
				return Observation{}, err
			}
			any, err := readAttributeFields(a.data)
			if err != nil {
				return Observation{}, err
			}
			typ, err := any.one(1, protowire.BytesType, true)
			if err != nil || string(typ.data) != attributesTypeURL {
				return Observation{}, failure(ProviderChanged, 200)
			}
			domain, err := any.one(2, protowire.BytesType, true)
			if err != nil {
				return Observation{}, err
			}
			o, err := decodeAttributesDomain(domain.data, id, now)
			if err != nil {
				return Observation{}, err
			}
			if selected != nil {
				return Observation{}, failure(ProviderChanged, 200)
			}
			selected = &o
		}
	}
	if selected == nil {
		return Observation{}, failure(AnalysisUnavailable, 200)
	}
	return *selected, nil
}

func decodeAttributesDomain(raw []byte, id string, now time.Time) (Observation, error) {
	f, err := readAttributeFields(raw)
	if err != nil {
		return Observation{}, err
	}
	t := track{}
	if len(f[1]) > 0 {
		b, err := f.one(1, protowire.Fixed64Type, true)
		if err != nil {
			return Observation{}, err
		}
		bpm := math.Float64frombits(b.number)
		if !finite(&bpm) || bpm <= 0 {
			return Observation{}, failure(ProviderChanged, 200)
		}
		t.Tempo = &bpm
	}
	var providerCamelot string
	if len(f[2]) > 0 {
		k, err := f.one(2, protowire.BytesType, true)
		if err != nil {
			return Observation{}, err
		}
		key, err := readAttributeFields(k.data)
		if err != nil {
			return Observation{}, err
		}
		name, err := key.one(1, protowire.BytesType, false)
		if err != nil {
			return Observation{}, err
		}
		mode, err := key.one(2, protowire.VarintType, false)
		if err != nil {
			return Observation{}, err
		}
		if len(name.data) > 0 && mode.number != 0 {
			tonic, ok := map[string]int{"C": 0, "C#": 1, "Db": 1, "D": 2, "D#": 3, "Eb": 3, "E": 4, "F": 5, "F#": 6, "Gb": 6, "G": 7, "G#": 8, "Ab": 8, "A": 9, "A#": 10, "Bb": 10, "B": 11}[string(name.data)]
			if !ok || mode.number > 2 {
				return Observation{}, failure(ProviderChanged, 200)
			}
			normalizedMode := int(mode.number) - 1
			t.Key = &tonic
			t.Mode = &normalizedMode
		}
		if len(key[3]) > 0 {
			c, err := key.one(3, protowire.BytesType, true)
			if err != nil {
				return Observation{}, err
			}
			camelot, err := readAttributeFields(c.data)
			if err != nil {
				return Observation{}, err
			}
			value, err := camelot.one(1, protowire.BytesType, false)
			if err != nil {
				return Observation{}, err
			}
			providerCamelot = string(value.data)
		}
	}
	o, err := normalize(id, payload{Track: &t}, now)
	if err != nil {
		return Observation{}, err
	}
	if providerCamelot != "" && (o.Camelot == nil || *o.Camelot != providerCamelot) {
		return Observation{}, failure(ProviderChanged, 200)
	}
	o.SourceEndpoint = AttributesV2
	return o, nil
}
