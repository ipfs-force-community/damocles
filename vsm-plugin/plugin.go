package vsmplugin

import (
	"context"
	"os"
	"path/filepath"
	goplugin "plugin"
	"strings"
)

type Plugin struct {
	*Manifest
	library *goplugin.Plugin
	State   State
}

// State present the state of plugin.
type State uint8

const (
	// Uninitialized indicates plugin is uninitialized.
	Uninitialized State = iota
	// Ready indicates plugin is ready to work.
	Ready
	// Dying indicates plugin will be close soon.
	Dying
	// Disable indicate plugin is disabled.
	Disable
)

func (s State) String() (str string) {
	switch s {
	case Uninitialized:
		str = "Uninitialized"
	case Ready:
		str = "Ready"
	case Dying:
		str = "Dying"
	case Disable:
		str = "Disable"
	}
	return
}

// LoadedPlugins collects loaded plugins info.
type LoadedPlugins struct {
	plugins map[Kind][]*Plugin
}

// add adds a plugin to loaded plugin collection.
func (p LoadedPlugins) add(plugin *Plugin) {
	plugins, ok := p.plugins[plugin.Kind]
	if !ok {
		plugins = make([]*Plugin, 0)
	}
	plugins = append(plugins, plugin)
	p.plugins[plugin.Kind] = plugins
}

// Get finds and returns plugin by kind and name parameters.
func (p LoadedPlugins) Get(kind Kind, name string) *Plugin {
	for _, plugin := range p.plugins[kind] {
		if plugin.State == Ready && plugin.Name == name {
			return plugin
		}
	}
	return nil
}

func (p LoadedPlugins) Foreach(kind Kind, fn func(plugin *Plugin) error) error {
	for _, plugin := range p.plugins[kind] {
		if plugin.State != Ready {
			continue
		}
		if err := fn(plugin); err != nil {
			return err
		}
	}
	return nil
}

func (p LoadedPlugins) ForeachAllKind(fn func(plugin *Plugin) error) error {
	for kind := range p.plugins {
		if err := p.Foreach(kind, fn); err != nil {
			return err
		}
	}
	return nil
}

// Init initializes the loaded plugin
// This method must be called after `Load` but before any other plugin method call.
func (p LoadedPlugins) Init(ctx context.Context, onError func(*Plugin, error)) {
	for _, plugins := range p.plugins {
		for _, plugin := range plugins {
			if plugin.OnInit != nil {
				if err := plugin.OnInit(ctx, plugin.Manifest); err != nil {
					onError(plugin, err)
					plugin.State = Disable
					continue
				}
			}
			plugin.State = Ready
		}
	}
}

// Shutdown cleanups all plugin resources.
// Notice: it just cleanups the resource of plugin, but cannot unload plugins(limited by go plugin).
func (p LoadedPlugins) Shutdown(ctx context.Context, onError func(*Plugin, error)) {
	for _, plugins := range p.plugins {
		for _, plugin := range plugins {
			plugin.State = Dying
			if plugin.OnShutdown == nil {
				continue
			}
			if err := plugin.OnShutdown(ctx, plugin.Manifest); err != nil {
				onError(plugin, err)
			}
		}
	}
}

func Load(pluginDir string) (loadedPlugins *LoadedPlugins, err error) {
	loadedPlugins = &LoadedPlugins{
		plugins: make(map[Kind][]*Plugin),
	}
	if pluginDir == "" {
		return
	}

	var entries []os.DirEntry
	entries, err = os.ReadDir(pluginDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), PluginSuffix) {
			continue
		}
		plugin := &Plugin{
			State: Uninitialized,
		}
		plugin.library, err = goplugin.Open(filepath.Join(pluginDir, entry.Name()))
		if err != nil {
			return
		}
		var manifestSym goplugin.Symbol
		manifestSym, err = plugin.library.Lookup(ManifestSymbol)
		if err != nil {
			return
		}
		manifestFn, ok := manifestSym.(func() *Manifest)
		if !ok {
			err = ErrInvalidPluginManifest
			return
		}
		plugin.Manifest = manifestFn()
		loadedPlugins.add(plugin)
	}
	return
}
