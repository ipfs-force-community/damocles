package plugin

import (
	"fmt"

	vsmplugin "github.com/ipfs-force-community/venus-cluster/vsm-plugin"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var plog = logging.New("objstore").With("driver", "plugin")

func OpenPluginObjStore(pluginName string, cfg objstore.Config, loadedPlugins *vsmplugin.LoadedPlugins) (objstore.Store, error) {
	p := loadedPlugins.Get(vsmplugin.ObjStore, pluginName)
	if p == nil {
		return nil, fmt.Errorf("objstore plugin not found: %s", pluginName)
	}
	objstore := vsmplugin.DeclareObjStoreManifest(p.Manifest)
	if objstore.Constructor == nil {
		return nil, fmt.Errorf("objstore plugin Constructor cannot be nil: %s", pluginName)
	}
	plog.With("plugin_name", pluginName)
	return objstore.Constructor(cfg)
}
