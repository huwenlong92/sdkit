package email

import (
	"io/fs"

	pkgemail "github.com/huwenlong92/sdkit/pkg/email"
)

func LoadTemplates(fsys fs.FS, templates map[string]Template) (map[string]Template, error) {
	return pkgemail.LoadTemplates(fsys, templates)
}
