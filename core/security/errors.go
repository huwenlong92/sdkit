package security

const (
	ErrCodeSecurityInternal = 4600
	ErrCodeRiskBlocked      = 4601
	ErrCodeCaptchaRequired  = 4602
	ErrCodeVerifyRequired   = 4603
	ErrCodeInvalidSign      = 4604
	ErrCodeNonceRequired    = 4605
	ErrCodeNonceReplay      = 4606
)

const (
	MsgSecurityInternal = "security check failed"
	MsgRiskBlocked      = "security blocked"
	MsgCaptchaRequired  = "captcha required"
	MsgVerifyRequired   = "verify required"
	MsgInvalidSign      = "invalid signature"
	MsgNonceRequired    = "missing nonce"
	MsgNonceReplay      = "nonce replay"
)
