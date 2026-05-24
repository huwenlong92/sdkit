package security

type Config struct {
	NetworkDisabled bool
	ReadonlyRootfs  bool
	CapDrop         []string
	NoNewPrivileges bool
	SeccompProfile  string
	Tmpfs           map[string]string
}

func Default() Config {
	return Config{
		NetworkDisabled: true,
		ReadonlyRootfs:  true,
		CapDrop:         []string{"ALL"},
		NoNewPrivileges: true,
		SeccompProfile:  DefaultSeccompProfile(),
		Tmpfs: map[string]string{
			"/tmp":     "rw,noexec,nosuid,size=64m",
			"/run":     "rw,noexec,nosuid,size=16m",
			"/var/tmp": "rw,noexec,nosuid,size=64m",
		},
	}
}
