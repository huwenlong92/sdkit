package jsonx

import "github.com/bytedance/sonic"

func Marshal(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	return sonic.Unmarshal(data, v)
}

func MarshalString(v any) (string, error) {
	return sonic.MarshalString(v)
}

func Valid(data []byte) bool {
	return sonic.Valid(data)
}
