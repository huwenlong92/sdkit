package custom

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

func init() {
	Register(Rule{
		Tag: "username",
		Validate: func(fl validator.FieldLevel) bool {
			return regexp.MustCompile(`^[a-zA-Z0-9_]{5,16}$`).MatchString(fl.Field().String())
		},
		Translate: "{0}格式不正确，需要5-16位字母、数字或下划线",
	})
}
