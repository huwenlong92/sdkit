package crontab

func New(opts ManagerOptions) (*Manager, error) {
	return NewManager(opts)
}
