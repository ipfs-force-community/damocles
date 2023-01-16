package kvstore

import (
	"fmt"

	vsmplugin "github.com/ipfs-force-community/venus-cluster/vsm-plugin"
)

var PluginLog = Log.With("driver", "plugin")

func OpenPluginDB(pluginName string, meta map[string]string, loadedPlugins *vsmplugin.LoadedPlugins) (DB, error) {
	p := loadedPlugins.Get(vsmplugin.KVStore, pluginName)
	if p == nil {
		return nil, fmt.Errorf("kvstore plugin not found: %s", pluginName)
	}
	db := vsmplugin.DeclareKVStoreManifest(p.Manifest)
	if db.Constructor == nil {
		return nil, fmt.Errorf("kvstore plugin Constructor cannot be nil: %s", pluginName)
	}
	PluginLog.With("plugin_name", pluginName)
	return db.Constructor(meta)
}
