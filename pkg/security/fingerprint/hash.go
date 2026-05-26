package fingerprint

import "github.com/huwenlong92/sdkit/pkg/security/crypto"

func Hash(info RequestInfo) string {
	return crypto.SHA256Hex([]byte(info.IP + "|" + info.UA + "|" + info.DeviceID))
}
