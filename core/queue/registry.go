package queue

type Registration struct {
	Pattern  string
	Handlers []any
}

type Registry struct {
	worker  Worker
	runtime *RegistryRuntime
}

func Register(pattern string, handlers ...any) Registration {
	return Registration{
		Pattern:  pattern,
		Handlers: append([]any(nil), handlers...),
	}
}

func NewRegistry(worker Worker) *Registry {
	return NewRegistryWithDispatcher(worker, NewDispatcher())
}

func NewRegistryWithDispatcher(worker Worker, dispatcher *Dispatcher) *Registry {
	return NewRegistryWithRuntime(worker, NewRegistryRuntime(dispatcher))
}

func NewRegistryWithRuntime(worker Worker, runtime *RegistryRuntime) *Registry {
	if runtime == nil {
		runtime = NewRegistryRuntime(nil)
	}
	return &Registry{worker: worker, runtime: runtime}
}

func (r *Registry) Use(middlewares ...Middleware) {
	if r == nil || r.worker == nil || r.runtime == nil {
		return
	}
	r.runtime.Use(middlewares...)
}

func (r *Registry) UseRuntime(middlewares ...RuntimeMiddleware) {
	if r == nil || r.worker == nil || r.runtime == nil {
		return
	}
	r.runtime.UseRuntime(middlewares...)
}

func (r *Registry) AddHook(h Hook) {
	if r == nil || r.runtime == nil {
		return
	}
	r.runtime.AddHook(h)
}

func (r *Registry) Register(pattern string, handlers ...any) error {
	if r == nil || r.worker == nil {
		return ErrNotInitialized
	}
	if err := r.runtime.Register(pattern, handlers...); err != nil {
		return err
	}
	r.worker.Handle(pattern, r.runtime.Handler(pattern))
	return nil
}

func (r *Registry) RegisterAll(registrations ...Registration) error {
	for _, registration := range registrations {
		if err := r.Register(registration.Pattern, registration.Handlers...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) Dispatcher() *Dispatcher {
	if r == nil {
		return nil
	}
	return r.runtime.Dispatcher()
}

func (r *Registry) Runtime() *RegistryRuntime {
	if r == nil {
		return nil
	}
	return r.runtime
}

func (r *Registry) Entries() []HandlerMetadata {
	if r == nil || r.runtime == nil {
		return nil
	}
	return r.runtime.Entries()
}

func (r *Registry) Metadata() RegistryMetadata {
	if r == nil || r.runtime == nil {
		return RegistryMetadata{}
	}
	return r.runtime.Metadata()
}
