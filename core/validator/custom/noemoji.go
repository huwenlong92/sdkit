package custom

import "github.com/go-playground/validator/v10"

func init() {
	Register(Rule{
		Tag: "noemoji",
		Validate: func(fl validator.FieldLevel) bool {
			for _, r := range fl.Field().String() {
				if isEmoji(r) {
					return false
				}
			}
			return true
		},
		Translate: "{0}不能包含 emoji 字符",
	})
}

func isEmoji(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0x200D) // zero-width joiner and above
}
