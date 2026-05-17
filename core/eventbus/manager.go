package eventbus

import "sync"

type Manager struct {
	mu    sync.RWMutex
	buses map[string]Service
}

func NewManager() *Manager {
	return &Manager{buses: make(map[string]Service)}
}

func (m *Manager) Register(name string, bus Service) error {
	if name == "" || bus == nil {
		return ErrBusNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buses[name] = bus
	return nil
}

func (m *Manager) Get(name string) (Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	bus := m.buses[name]
	if bus == nil {
		return nil, ErrBusNotFound
	}
	return bus, nil
}

func (m *Manager) MustGet(name string) Service {
	bus, _ := m.Get(name)
	return bus
}

func (m *Manager) Close() error {
	m.mu.Lock()
	buses := make([]Service, 0, len(m.buses))
	for _, bus := range m.buses {
		buses = append(buses, bus)
	}
	m.buses = make(map[string]Service)
	m.mu.Unlock()

	var firstErr error
	for _, bus := range buses {
		if err := bus.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
