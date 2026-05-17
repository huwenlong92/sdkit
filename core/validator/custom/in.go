package custom

import (
	"strings"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

func init() {
	Register(Rule{
		Tag: "in",
		Validate: func(fl validator.FieldLevel) bool {
			val := fl.Field().String()
			options := strings.Split(fl.Param(), ",")
			for _, opt := range options {
				if strings.TrimSpace(opt) == val {
					return true
				}
			}
			return false
		},
		Translate: "{0}必须是 [{1}] 中的一个",
		TranslateFunc: func(ut ut.Translator, fe validator.FieldError) string {
			options := strings.Split(fe.Param(), ",")
			for i, opt := range options {
				options[i] = strings.TrimSpace(opt)
			}
			t, _ := ut.T("in", fe.Field(), strings.Join(options, ", "))
			return t
		},
	})
}
