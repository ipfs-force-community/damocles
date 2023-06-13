package kvstore

import (
	"fmt"

	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"
)

var PluginLog = Log.With("driver", "plugin")

func OpenPluginDB(pluginName string, meta map[string]string, loadedPlugins *managerplugin.LoadedPlugins) (DB, error) {
	p := loadedPlugins.Get(managerplugin.KVStore, pluginName)
	if p == nil {
		return nil, fmt.Errorf("kvstore plugin not found: %s", pluginName)
	}
	db := managerplugin.DeclareKVStoreManifest(p.Manifest)
	if db.Constructor == nil {
		return nil, fmt.Errorf("kvstore plugin Constructor cannot be nil: %s", pluginName)
	}
	PluginLog.With("plugin_name", pluginName)
	return db.Constructor(meta)
}
