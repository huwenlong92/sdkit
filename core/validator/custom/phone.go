package custom

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

func init() {
	// 中国大陆固定电话：区号-号码，如 010-12345678
	Register(Rule{
		Tag: "phone",
		Validate: func(fl validator.FieldLevel) bool {
			return regexp.MustCompile(`^0\d{2,3}-?\d{7,8}$`).MatchString(fl.Field().String())
		},
		Translate: "{0}固定电话格式不正确",
	})
}
