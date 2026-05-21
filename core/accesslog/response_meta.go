package accesslog

import (
	"bytes"
	"encoding/json"
	"strconv"
)

func responseMeta(body []byte) (int, string) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return 0, ""
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return 0, ""
	}

	var (
		errCode    int
		hasErrCode bool
		msg        string
	)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			break
		}
		key, ok := keyToken.(string)
		if !ok {
			break
		}

		switch key {
		case "err_code", "code":
			value, ok := decodeResponseValue(decoder)
			if !ok {
				return errCode, msg
			}
			if code, ok := responseInt(value); ok {
				errCode = code
				hasErrCode = true
			}
		case "msg", "message":
			value, ok := decodeResponseValue(decoder)
			if !ok {
				return errCode, msg
			}
			if text, ok := value.(string); ok {
				msg = text
			}
		default:
			if !discardResponseValue(decoder) {
				return errCode, msg
			}
		}
		if hasErrCode && msg != "" {
			return errCode, msg
		}
	}
	if !hasErrCode {
		return 0, msg
	}
	return errCode, msg
}

func decodeResponseValue(decoder *json.Decoder) (any, bool) {
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

func discardResponseValue(decoder *json.Decoder) bool {
	var value json.RawMessage
	return decoder.Decode(&value) == nil
}

func responseInt(value any) (int, bool) {
	switch v := value.(type) {
	case json.Number:
		code, err := strconv.Atoi(v.String())
		if err == nil {
			return code, true
		}
	case float64:
		return int(v), true
	case string:
		code, err := strconv.Atoi(v)
		if err == nil {
			return code, true
		}
	}
	return 0, false
}
