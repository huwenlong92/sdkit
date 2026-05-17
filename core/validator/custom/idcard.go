package custom

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

func init() {
	// 中国大陆 18 位身份证号（简单校验格式，不校验校验位）
	Register(Rule{
		Tag: "idcard",
		Validate: func(fl validator.FieldLevel) bool {
			return regexp.MustCompile(`^[1-9]\d{5}(18|19|20)?\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]$`).MatchString(fl.Field().String())
		},
		Translate: "{0}身份证号格式不正确",
	})
}
