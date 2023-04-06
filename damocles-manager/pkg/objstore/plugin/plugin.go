package plugin

import (
	"fmt"

	managerplugin "github.com/ipfs-force-community/damocles/damocles-manager-plugin"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
)

var plog = logging.New("objstore").With("driver", "plugin")

func OpenPluginObjStore(pluginName string, cfg objstore.Config, loadedPlugins *managerplugin.LoadedPlugins) (objstore.Store, error) {
	p := loadedPlugins.Get(managerplugin.ObjStore, pluginName)
	if p == nil {
		return nil, fmt.Errorf("objstore plugin not found: %s", pluginName)
	}
	objstore := managerplugin.DeclareObjStoreManifest(p.Manifest)
	if objstore.Constructor == nil {
		return nil, fmt.Errorf("objstore plugin Constructor cannot be nil: %s", pluginName)
	}
	plog.With("plugin_name", pluginName)
	return objstore.Constructor(cfg)
}
