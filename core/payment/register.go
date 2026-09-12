package payment

// RegisterProvider adds a provider to the running service. Duplicate names are
// rejected so that an in-flight operation cannot silently change implementation.
func (s *Service) RegisterProvider(adapter ProviderAdapter) error {
	return s.registry.Register(adapter)
}

// RegisterProvider registers against the initialized default service.
func RegisterProvider(adapter ProviderAdapter) error {
	service, err := Default()
	if err != nil {
		return err
	}
	return service.RegisterProvider(adapter)
}
