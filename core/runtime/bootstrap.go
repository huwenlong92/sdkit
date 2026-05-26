package runtime

const (
	CapabilityBootstrap      = "bootstrap"
	CapabilityBootstrapError = "bootstrap.error"
)

func RequireBootstrap() Dependency {
	return Require(CapabilityBootstrap)
}

func OptionalBootstrap() Dependency {
	return Optional(CapabilityBootstrap)
}
