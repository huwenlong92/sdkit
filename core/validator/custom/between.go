package custom

import (
	"strconv"
	"strings"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

func init() {
	Register(Rule{
		Tag: "between",
		Validate: func(fl validator.FieldLevel) bool {
			val := fl.Field().Float()
			parts := strings.Split(fl.Param(), ",")
			if len(parts) != 2 {
				return false
			}
			min, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			max, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 != nil || err2 != nil || min > max {
				return false
			}
			return val >= min && val <= max
		},
		Translate: "{0}必须在{1}到{2}之间",
		TranslateFunc: func(ut ut.Translator, fe validator.FieldError) string {
			parts := strings.Split(fe.Param(), ",")
			if len(parts) == 2 {
				t, _ := ut.T("between", fe.Field(), strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				return t
			}
			return fe.Field() + "区间校验失败"
		},
	})
}
