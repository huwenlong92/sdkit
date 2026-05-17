package custom

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

func init() {
	Register(Rule{
		Tag: "mobile",
		Validate: func(fl validator.FieldLevel) bool {
			return regexp.MustCompile(`^1[3-9]\d{9}$`).MatchString(fl.Field().String())
		},
		Translate: "{0}手机号格式不正确",
	})
}
