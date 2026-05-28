package email

import (
	"fmt"
	"io/fs"
	"strings"
)

func LoadTemplates(fsys fs.FS, templates map[string]Template) (map[string]Template, error) {
	if len(templates) == 0 {
		return nil, nil
	}
	loaded := make(map[string]Template, len(templates))
	for name, tpl := range templates {
		resolved, err := loadTemplateFiles(fsys, name, tpl)
		if err != nil {
			return nil, err
		}
		loaded[name] = resolved
	}
	return loaded, nil
}

func loadTemplateFiles(fsys fs.FS, name string, tpl Template) (Template, error) {
	var err error
	if tpl.Subject, err = loadTemplateField(fsys, name, "subject_file", tpl.Subject, tpl.SubjectFile); err != nil {
		return Template{}, err
	}
	if tpl.Text, err = loadTemplateField(fsys, name, "text_file", tpl.Text, tpl.TextFile); err != nil {
		return Template{}, err
	}
	if tpl.HTML, err = loadTemplateField(fsys, name, "html_file", tpl.HTML, tpl.HTMLFile); err != nil {
		return Template{}, err
	}
	return tpl, nil
}

func loadTemplateField(fsys fs.FS, name string, field string, value string, file string) (string, error) {
	file = strings.TrimSpace(file)
	if file == "" {
		return value, nil
	}
	if fsys == nil {
		return "", ErrTemplateFSRequired
	}
	if !fs.ValidPath(file) {
		return "", fmt.Errorf("email: template %s %s has invalid path: %s", name, field, file)
	}
	content, err := fs.ReadFile(fsys, file)
	if err != nil {
		return "", fmt.Errorf("email: load template %s %s %s: %w", name, field, file, err)
	}
	return string(content), nil
}
