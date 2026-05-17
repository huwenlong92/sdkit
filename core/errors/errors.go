package errors

import "fmt"

const (
	CodeOK           = 200
	CodeBadRequest   = 400
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeConflict     = 409
	CodeInternal     = 500

	CodeBusinessError       = 3000
	CodeBusinessUnavailable = 3001
	CodeBusinessDuplicated  = 3002
	CodeBusinessProtected   = 3003
	CodeBusinessConflict    = 3004

	CodeAuthRequired      = 4001
	CodeOperationFailed   = 5001
	CodeQueueTaskConflict = 4091
)

const (
	SubCodeBadRequest          = "BAD_REQUEST"
	SubCodeValidation          = "VALIDATION_ERROR"
	SubCodeJSONType            = "JSON_TYPE_ERROR"
	SubCodeUnauthorized        = "UNAUTHORIZED"
	SubCodeForbidden           = "FORBIDDEN"
	SubCodeNotFound            = "NOT_FOUND"
	SubCodeConflict            = "CONFLICT"
	SubCodeInternal            = "INTERNAL_ERROR"
	SubCodeInvalidCredentials  = "INVALID_CREDENTIALS"
	SubCodeAccountDisabled     = "ACCOUNT_DISABLED"
	SubCodeLoginFailed         = "LOGIN_FAILED"
	SubCodeUserNotFound        = "USER_NOT_FOUND"
	SubCodeRoleQueryFailed     = "ROLE_QUERY_FAILED"
	SubCodeSecurityCheckFailed = "SECURITY_CHECK_FAILED"
	SubCodeBusinessError       = "BUSINESS_ERROR"
	SubCodeBusinessUnavailable = "BUSINESS_UNAVAILABLE"
	SubCodeBusinessDuplicated  = "BUSINESS_DUPLICATED"
	SubCodeBusinessProtected   = "BUSINESS_PROTECTED"
	SubCodeBusinessConflict    = "BUSINESS_CONFLICT"
	SubCodeAuthRequired        = "AUTH_REQUIRED"
	SubCodeOperationFailed     = "OPERATION_FAILED"
	SubCodeQueueTaskConflict   = "QUEUE_TASK_CONFLICT"
)

type AppError struct {
	Code    int
	SubCode string
	Message string
	Data    interface{}
	Err     error
}

func (e *AppError) Error() string {
	return fmt.Sprintf("code=%d sub_code=%s message=%s", e.Code, e.SubCode, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code && e.SubCode == t.SubCode
}

func New(code int, subCode, message string) *AppError {
	return &AppError{Code: code, SubCode: subCode, Message: message}
}

func NewWithData(code int, subCode, message string, data interface{}) *AppError {
	return &AppError{Code: code, SubCode: subCode, Message: message, Data: data}
}

func NewCode(code int, message string) *AppError {
	return New(code, SubCodeForCode(code), message)
}

func NewCodeWithData(code int, message string, data interface{}) *AppError {
	return NewWithData(code, SubCodeForCode(code), message, data)
}

func Wrap(err error, code int, subCode, message string) *AppError {
	return &AppError{Code: code, SubCode: subCode, Message: message, Err: err}
}

func WrapWithData(err error, code int, subCode, message string, data interface{}) *AppError {
	return &AppError{Code: code, SubCode: subCode, Message: message, Data: data, Err: err}
}

func SubCodeForCode(code int) string {
	switch code {
	case CodeBadRequest:
		return SubCodeBadRequest
	case CodeUnauthorized:
		return SubCodeUnauthorized
	case CodeForbidden:
		return SubCodeForbidden
	case CodeNotFound:
		return SubCodeNotFound
	case CodeConflict:
		return SubCodeConflict
	case CodeInternal:
		return SubCodeInternal
	case CodeBusinessError:
		return SubCodeBusinessError
	case CodeBusinessUnavailable:
		return SubCodeBusinessUnavailable
	case CodeBusinessDuplicated:
		return SubCodeBusinessDuplicated
	case CodeBusinessProtected:
		return SubCodeBusinessProtected
	case CodeBusinessConflict:
		return SubCodeBusinessConflict
	case CodeAuthRequired:
		return SubCodeAuthRequired
	case CodeOperationFailed:
		return SubCodeOperationFailed
	case CodeQueueTaskConflict:
		return SubCodeQueueTaskConflict
	default:
		return fmt.Sprintf("CODE_%d", code)
	}
}

var (
	ErrBadRequest     = New(CodeBadRequest, SubCodeBadRequest, "请求参数错误")
	ErrUnauthorized   = New(CodeUnauthorized, SubCodeUnauthorized, "未授权")
	ErrForbidden      = New(CodeForbidden, SubCodeForbidden, "禁止访问")
	ErrNotFound       = New(CodeNotFound, SubCodeNotFound, "资源不存在")
	ErrConflict       = New(CodeConflict, SubCodeConflict, "资源冲突")
	ErrInternalServer = New(CodeInternal, SubCodeInternal, "服务器内部错误")
)
