package response

import (
	stderrors "errors"
	"net/http"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/jsonx"

	"github.com/gin-gonic/gin"
)

const (
	SuccessCode = apperrors.CodeOK
	SuccessMsg  = "ok"
	contentType = "application/json; charset=utf-8"
)

type Body struct {
	ErrCode int         `json:"err_code"`
	Msg     string      `json:"msg"`
	Data    interface{} `json:"data"`
}

func JSON(c *gin.Context, code int, data any) {
	b, err := jsonx.Marshal(data)
	if err != nil {
		c.Data(http.StatusInternalServerError, contentType, []byte(`{"err_code":500,"msg":"json marshal error","data":null}`))
		c.Abort()
		return
	}
	c.Data(code, contentType, b)
}

func AbortJSON(c *gin.Context, code int, data any) {
	JSON(c, code, data)
	c.Abort()
}

func Success(c *gin.Context, data interface{}) {
	JSON(c, http.StatusOK, Body{
		ErrCode: SuccessCode,
		Msg:     SuccessMsg,
		Data:    data,
	})
}

func Error(c *gin.Context, err error) {
	appErr := AppError(err)
	JSON(c, http.StatusOK, Body{
		ErrCode: appErr.Code,
		Msg:     appErr.Message,
		Data:    appErr.Data,
	})
}

func Fail(c *gin.Context, err error) {
	Error(c, err)
}

func AbortError(c *gin.Context, err error) {
	Error(c, err)
	c.Abort()
}

func AbortFail(c *gin.Context, err error) {
	Fail(c, err)
	c.Abort()
}

func AppError(err error) *apperrors.AppError {
	if err == nil {
		return apperrors.ErrInternalServer
	}

	var appErr *apperrors.AppError
	if stderrors.As(err, &appErr) {
		return appErr
	}
	return apperrors.Wrap(err, apperrors.CodeInternal, apperrors.SubCodeInternal, apperrors.ErrInternalServer.Message)
}
