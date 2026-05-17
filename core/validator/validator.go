package validator

import (
	"encoding/json"
	stderrors "errors"
	"reflect"
	"strings"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
	"github.com/huwenlong92/sdkit/core/response"
	"github.com/huwenlong92/sdkit/core/validator/custom"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zhTranslations "github.com/go-playground/validator/v10/translations/zh"
)

var (
	trans ut.Translator
)

func Init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(field reflect.StructField) string {
			name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})

		zhT := zh.New()
		uni := ut.New(zhT, zhT)
		trans, _ = uni.GetTranslator("zh")
		zhTranslations.RegisterDefaultTranslations(v, trans)

		custom.RegisterAll(v, trans)
	}
}

func HandlerValidatorError(c *gin.Context, err error) {
	response.Error(c, ToAppError(err))
}

func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindBodyWith(dst, binding.JSON); err != nil {
		return ToAppError(err)
	}
	return nil
}

func BindQuery(c *gin.Context, dst any) error {
	if err := c.ShouldBindQuery(dst); err != nil {
		return ToAppError(err)
	}
	return nil
}

func BindForm(c *gin.Context, dst any) error {
	if err := c.ShouldBindWith(dst, binding.Form); err != nil {
		return ToAppError(err)
	}
	return nil
}

func ToAppError(err error) error {
	var errs validator.ValidationErrors
	if stderrors.As(err, &errs) {
		for _, e := range errs {
			return apperrors.Wrap(err, apperrors.CodeBadRequest, apperrors.SubCodeValidation, translateFieldError(e))
		}
	}

	var typeErr *json.UnmarshalTypeError
	if stderrors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			field = "字段"
		}
		return apperrors.Wrap(err, apperrors.CodeBadRequest, apperrors.SubCodeJSONType, field+"类型错误")
	}

	return apperrors.Wrap(err, apperrors.CodeBadRequest, apperrors.SubCodeBadRequest, "参数错误")
}

func translateFieldError(err validator.FieldError) string {
	if trans != nil {
		return err.Translate(trans)
	}
	return err.Error()
}
