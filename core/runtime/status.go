package runtime

import (
	"fmt"
	"sync"
)

type Status string

const (
	StatusBooting      Status = "booting"
	StatusRunning      Status = "running"
	StatusStopping     Status = "stopping"
	StatusShuttingDown Status = StatusStopping
	StatusStopped      Status = "stopped"
	StatusFailed       Status = "failed"
)

type Health struct {
	Name   string
	Status Status
	Error  error
}

type runtimeStatus struct {
	mu     sync.RWMutex
	status Status
	err    error
}

func newRuntimeStatus() *runtimeStatus {
	return &runtimeStatus{status: StatusStopped}
}

func (s *runtimeStatus) Status() Status {
	if s == nil {
		return StatusStopped
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return normalizeStatus(s.status)
}

func (s *runtimeStatus) Set(status Status, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.status = normalizeStatus(status)
	s.err = err
	s.mu.Unlock()
}

func (s *runtimeStatus) Health(name string) Health {
	if s == nil {
		return Health{Name: name, Status: StatusStopped}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Health{Name: name, Status: normalizeStatus(s.status), Error: s.err}
}

func normalizeStatus(status Status) Status {
	if status == "" {
		return StatusStopped
	}
	return status
}

type runtimeStatusController interface {
	setRuntimeStatus(Status, error)
	runtimeHealth(string) Health
}

type runtimeCapability struct {
	CapabilityContract
	state *runtimeStatus
}

func newRuntimeCapability(capability CapabilityContract) Capability {
	if capability == nil {
		return nil
	}
	if c, ok := capability.(Capability); ok {
		if _, ok := c.(runtimeStatusController); ok {
			return c
		}
	}
	return &runtimeCapability{
		CapabilityContract: capability,
		state:              newRuntimeStatus(),
	}
}

func (c *runtimeCapability) Status() Status {
	if c == nil {
		return StatusStopped
	}
	return c.state.Status()
}

func (c *runtimeCapability) setRuntimeStatus(status Status, err error) {
	if c == nil {
		return
	}
	c.state.Set(status, err)
}

func (c *runtimeCapability) runtimeHealth(name string) Health {
	if c == nil {
		return Health{Name: name, Status: StatusStopped}
	}
	return c.state.Health(name)
}

type runtimeProvider struct {
	ProviderContract
	state *runtimeStatus
}

func newRuntimeProvider(provider ProviderContract) Provider {
	if provider == nil {
		return nil
	}
	if p, ok := provider.(Provider); ok {
		if _, ok := p.(runtimeStatusController); ok {
			return p
		}
	}
	return &runtimeProvider{
		ProviderContract: provider,
		state:            newRuntimeStatus(),
	}
}

func (p *runtimeProvider) Status() Status {
	if p == nil {
		return StatusStopped
	}
	return p.state.Status()
}

func (p *runtimeProvider) Metadata() ProviderMetadata {
	if p == nil {
		return ProviderMetadata{Mode: ProviderModeJob}
	}
	return providerMetadata(p.ProviderContract)
}

func (p *runtimeProvider) ProviderMode() ProviderMode {
	if p == nil {
		return ProviderModeJob
	}
	return providerMode(p.ProviderContract)
}

func (p *runtimeProvider) RuntimeCapabilities() []CapabilityContract {
	if p == nil {
		return nil
	}
	return providerRuntimeCapabilities(p.ProviderContract)
}

func (p *runtimeProvider) setRuntimeStatus(status Status, err error) {
	if p == nil {
		return
	}
	p.state.Set(status, err)
}

func (p *runtimeProvider) runtimeHealth(name string) Health {
	if p == nil {
		return Health{Name: name, Status: StatusStopped}
	}
	return p.state.Health(name)
}

func setObjectStatus(object any, status Status, err error) {
	controller, ok := object.(runtimeStatusController)
	if !ok {
		return
	}
	controller.setRuntimeStatus(status, err)
}

func objectHealth(object any, name string) Health {
	controller, ok := object.(runtimeStatusController)
	if ok {
		return controller.runtimeHealth(name)
	}
	if state, ok := object.(interface{ Status() Status }); ok {
		return Health{Name: name, Status: normalizeStatus(state.Status())}
	}
	return Health{Name: name, Status: StatusStopped}
}

func (a *App) ProviderStatus(name string) Health {
	if a == nil || name == "" {
		return Health{Name: name, Status: StatusStopped}
	}
	provider, ok := a.Provider(name)
	if !ok {
		return Health{
			Name:   name,
			Status: StatusStopped,
			Error:  fmt.Errorf("%w: %s", ErrProviderNotFound, name),
		}
	}
	return objectHealth(provider, providerName(provider))
}

func (a *App) CapabilityStatus(name string) Health {
	if a == nil || name == "" {
		return Health{Name: name, Status: StatusStopped}
	}
	capability, ok := a.Capability(name)
	if !ok {
		return Health{
			Name:   name,
			Status: StatusStopped,
			Error:  fmt.Errorf("%w: %s", ErrCapabilityNotFound, name),
		}
	}
	return objectHealth(capability, capabilityName(capability))
}

func (a *App) ProviderStatuses() []Health {
	if a == nil {
		return nil
	}
	providers := a.Providers()
	out := make([]Health, 0, len(providers))
	for _, provider := range providers {
		out = append(out, objectHealth(provider, providerName(provider)))
	}
	return out
}

func (a *App) CapabilityStatuses() []Health {
	if a == nil {
		return nil
	}
	capabilities := a.Capabilities()
	out := make([]Health, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, objectHealth(capability, capabilityName(capability)))
	}
	return out
}

func (a *App) resetRuntimeStatuses() {
	if a == nil {
		return
	}
	for _, capability := range a.Capabilities() {
		setObjectStatus(capability, StatusStopped, nil)
	}
	for _, provider := range a.Providers() {
		setObjectStatus(provider, StatusStopped, nil)
	}
}

func (a *App) failRuntimeStatuses(err error) {
	if a == nil {
		return
	}
	for _, capability := range a.Capabilities() {
		setObjectStatus(capability, StatusFailed, err)
	}
	for _, provider := range a.Providers() {
		setObjectStatus(provider, StatusFailed, err)
	}
}
