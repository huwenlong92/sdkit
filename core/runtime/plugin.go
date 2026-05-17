package runtime

import "errors"

var (
	ErrPluginNil           = errors.New("runtime: plugin is nil")
	ErrPluginNameRequired  = errors.New("runtime: plugin name is required")
	ErrPluginNameReserved  = errors.New("runtime: plugin name is reserved")
	ErrPluginNameDuplicate = errors.New("runtime: plugin name is duplicate")
	ErrPluginNotFound      = errors.New("runtime: plugin not found")
)

type PluginMetadata struct {
	Name        string
	Description string
	Group       string
	Internal    bool
}

type Plugin interface {
	Metadata() PluginMetadata
}

func NewPlugin(metadata PluginMetadata) Plugin {
	return metadataPlugin{metadata: metadata}
}

type metadataPlugin struct {
	metadata PluginMetadata
}

func (p metadataPlugin) Metadata() PluginMetadata {
	return p.metadata
}

func pluginMetadata(plugin Plugin) PluginMetadata {
	if plugin == nil {
		return PluginMetadata{}
	}
	return plugin.Metadata()
}

func isReservedPluginName(name string) bool {
	switch name {
	case "plugin", "default", "main":
		return true
	default:
		return false
	}
}

func pluginName(plugin Plugin) string {
	return pluginMetadata(plugin).Name
}
