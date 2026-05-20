package storage

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	corestorage "github.com/huwenlong92/sdkit/core/storage"
)

type Config = corestorage.Config
type StoreConfig = corestorage.StoreConfig
type Manager = corestorage.Manager
type FileSystem = corestorage.FileSystem
type Policy = corestorage.Policy

const (
	KeyStorage = corestorage.KeyStorage
	Name       = string(KeyStorage)
)

func FromDefault() (*Manager, error) {
	return corestorage.ManagerDefault()
}

func From(app *runtime.App) *Manager {
	return corestorage.From(app)
}
