package fingerprint

import securitycrypto "github.com/huwenlong92/sdkit/pkg/security/crypto"

func Hash(info RequestInfo) string {
	return securitycrypto.SHA256Hex([]byte(info.IP + "|" + info.UA + "|" + info.DeviceID))
}
