package custom

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

func init() {
	// 日期格式 YYYY-MM-DD
	Register(Rule{
		Tag: "date",
		Validate: func(fl validator.FieldLevel) bool {
			return regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(fl.Field().String())
		},
		Translate: "{0}日期格式不正确，应为 YYYY-MM-DD",
	})
}
