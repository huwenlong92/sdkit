package custom

import (
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

type Rule struct {
	Tag           string
	Validate      func(fl validator.FieldLevel) bool
	Translate     string                                           // 翻译模板，{0} 字段名 / {1}{2}... 额外参数
	TranslateFunc func(ut.Translator, validator.FieldError) string // 优先级高于 Translate
}

var rules []Rule

func Register(r Rule) {
	rules = append(rules, r)
}

func RegisterAll(v *validator.Validate, trans ut.Translator) {
	for _, r := range rules {
		v.RegisterValidation(r.Tag, r.Validate)
		v.RegisterTranslation(r.Tag, trans, func(ut ut.Translator) error {
			return ut.Add(r.Tag, r.Translate, true)
		}, func(ut ut.Translator, fe validator.FieldError) string {
			if r.TranslateFunc != nil {
				return r.TranslateFunc(ut, fe)
			}
			t, _ := ut.T(r.Tag, fe.Field())
			return t
		})
	}
}
