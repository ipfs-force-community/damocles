package plugin

import (
	"fmt"

	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/filestore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var plog = logging.New("filestore").With("driver", "plugin")

func OpenPluginFileStore(pluginName string, cfg filestore.Config, loadedPlugins *managerplugin.LoadedPlugins) (filestore.Store, error) {
	p := loadedPlugins.Get(managerplugin.FileStore, pluginName)
	if p == nil {
		return nil, fmt.Errorf("filestore plugin not found: %s", pluginName)
	}
	filestore := managerplugin.DeclareFileStoreManifest(p.Manifest)
	if filestore.Constructor == nil {
		return nil, fmt.Errorf("filestore plugin Constructor cannot be nil: %s", pluginName)
	}
	plog.With("plugin_name", pluginName)
	return filestore.Constructor(cfg)
}
