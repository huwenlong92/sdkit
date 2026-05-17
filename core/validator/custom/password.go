package custom

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

func init() {
	Register(Rule{
		Tag: "password",
		Validate: func(fl validator.FieldLevel) bool {
			s := fl.Field().String()
			if len(s) < 6 || len(s) > 20 {
				return false
			}
			hasLetter := regexp.MustCompile(`[a-zA-Z]`).MatchString(s)
			hasDigit := regexp.MustCompile(`\d`).MatchString(s)
			return hasLetter && hasDigit
		},
		Translate: "{0}需要6-20位，包含字母和数字",
	})
}
