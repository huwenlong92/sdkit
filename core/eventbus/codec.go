package eventbus

import "encoding/json"

type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

type JSONCodec struct{}

func (JSONCodec) Marshal(v any) ([]byte, error) {
	switch payload := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), payload...), nil
	case json.RawMessage:
		return append([]byte(nil), payload...), nil
	default:
		return json.Marshal(v)
	}
}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
