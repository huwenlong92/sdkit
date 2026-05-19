package ginresponder

import (
	stderrors "errors"
	"net/http"

	apperrors "github.com/huwenlong92/sdkit/core/errors"

	"github.com/gin-gonic/gin"
)

type ErrorResponder func(c *gin.Context, status int, err error)

func RespondError(responder ErrorResponder, c *gin.Context, status int, err error) {
	if responder == nil {
		responder = DefaultError
	}
	responder(c, normalizeStatus(status), err)
}

func DefaultError(c *gin.Context, status int, err error) {
	message := http.StatusText(http.StatusInternalServerError)
	if err != nil {
		message = err.Error()
		var appErr *apperrors.AppError
		if stderrors.As(err, &appErr) && appErr.Message != "" {
			message = appErr.Message
		}
	}
	c.JSON(normalizeStatus(status), gin.H{"error": message})
	c.Abort()
}

func normalizeStatus(status int) int {
	if status < 100 || status > 599 {
		return http.StatusInternalServerError
	}
	return status
}
