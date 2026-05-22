package crontab

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	templates map[string]Template
}

func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]Template),
	}
}

func (r *Registry) Register(t Template) error {
	t = normalizeTemplate(t)
	if t.Key == "" {
		return fmt.Errorf("crontab template key is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.templates[t.Key]; ok {
		return fmt.Errorf("crontab template duplicated: %s", t.Key)
	}

	r.templates[t.Key] = t
	return nil
}

func (r *Registry) MustRegister(t Template) {
	_ = r.Register(t)
}

func (r *Registry) RegisterAll(templates ...Template) error {
	for _, template := range templates {
		if err := r.Register(template); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTemplate(t Template) Template {
	if t.Key == "" {
		t.Key = t.Name
	}
	if t.Name == "" {
		t.Name = t.Key
	}
	return t
}

func (r *Registry) Get(name string) (Template, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.templates[name]
	return t, ok
}

func (r *Registry) List() []Template {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Template, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (r *Registry) ListTemplateInfo() []TemplateInfo {
	templates := r.List()
	out := make([]TemplateInfo, 0, len(templates))
	for _, tpl := range templates {
		out = append(out, TemplateInfo{
			Key:            tpl.Key,
			Name:           tpl.Key,
			Label:          tpl.Name,
			Desc:           tpl.Desc,
			Description:    tpl.Desc,
			Spec:           tpl.Spec,
			Mode:           string(ModeLocal),
			Enabled:        tpl.Enabled,
			AllowBuiltin:   tpl.Spec != "",
			AllowDB:        tpl.AllowDB,
			AllowOverlap:   tpl.AllowOverlap,
			LogDisabled:    tpl.LogDisabled,
			DefaultSpec:    tpl.Spec,
			DefaultPayload: tpl.DefaultPayload,
			PayloadFormat:  templatePayloadFormat(tpl),
			PayloadSchema:  tpl.PayloadSchema,
			Timeout:        templateTimeout(tpl),
			TimeoutSec:     int(templateTimeout(tpl).Seconds()),
		})
	}
	return out
}

func templatePayloadFormat(tpl Template) string {
	if tpl.PayloadFormat != "" {
		return tpl.PayloadFormat
	}
	return "raw"
}

func (r *Registry) ListDBTemplateInfo() []TemplateInfo {
	templates := r.ListTemplateInfo()
	out := make([]TemplateInfo, 0, len(templates))
	for _, tpl := range templates {
		if tpl.Enabled && tpl.AllowDB {
			out = append(out, tpl)
		}
	}
	return out
}

func (r *Registry) ValidateJob(job Job) (Template, error) {
	tpl, ok := r.Get(job.Name)
	if !ok {
		return Template{}, ErrTemplateNotFound
	}
	if !tpl.Enabled {
		return tpl, ErrTemplateDisabled
	}
	if (job.Source == SourceDynamic || job.Source == SourceDB) && !tpl.AllowDB {
		return tpl, ErrDynamicNotAllowed
	}
	if tpl.Handler == nil {
		return tpl, ErrLocalHandlerMissing
	}
	return tpl, nil
}

func (r *Registry) BuiltinJobs() []Job {
	templates := r.List()
	jobs := make([]Job, 0, len(templates))
	for _, tpl := range templates {
		job, ok := buildBuiltinJob(tpl)
		if ok {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return jobs
}

func buildBuiltinJob(tpl Template) (Job, bool) {
	if !tpl.Enabled || tpl.Spec == "" {
		return Job{}, false
	}

	timeout := templateTimeout(tpl)

	return Job{
		ID:      "builtin." + tpl.Key,
		Name:    tpl.Key,
		Label:   tpl.Name,
		Spec:    tpl.Spec,
		Source:  SourceBuiltin,
		Mode:    ModeLocal,
		Enabled: true,
		Timeout: timeout,
	}, true
}
