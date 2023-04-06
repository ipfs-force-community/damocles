package managerplugin

import (
	"context"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/ipfs-force-community/damocles/damocles-manager-plugin/kvstore"
	"github.com/ipfs-force-community/damocles/damocles-manager-plugin/objstore"
)

const (
	// PluginSuffix defines VSM plugin's file suffix.
	PluginSuffix = ".so"
	// ManifestSymbol defines VSM plugin's entrance symbol.
	// Plugin take manifest info from this symbol.
	ManifestSymbol = "PluginManifest"
)

var (
	ErrInvalidPluginManifest = fmt.Errorf("invalid plugin manifest")
)

// Kind presents the kind of plugin.
type Kind uint8

const (
	// KVStore indicates it is a KVStore plugin.
	KVStore Kind = 1 + iota
	// ObjStore indicates it is a ObjStore plugin.
	ObjStore
	// RegisterJsonRpc indicates it is a RegisterJsonRpc plugin.
	RegisterJsonRpc
	// SyncSectorState indicates it is a SyncSectorState plugin.
	SyncSectorState
)

func (k Kind) String() (str string) {
	switch k {
	case KVStore:
		str = "KVStore"
	case ObjStore:
		str = "ObjStore"
	case SyncSectorState:
		str = "SyncSectorState"
	}
	return
}

type Manifest struct {
	// The plugin name
	Name string
	// The description of plugin
	Description string
	BuildTime   string
	// OnInit defines the plugin init logic.
	// it will be called after VSM-daemon init.
	// return error will stop load plugin process and VSM startup.
	// `pluginsDir` is the vsm plugins directory
	OnInit func(ctx context.Context, pluginsDir string, manifest *Manifest) error
	// OnShutDown defines the plugin cleanup logic.
	// return error will write log and continue shutdown.
	OnShutdown func(ctx context.Context, manifest *Manifest) error

	Kind Kind
}

type ObjStoreManifest struct {
	Manifest

	Constructor func(cfg objstore.Config) (objstore.Store, error)
}

type KVStoreManifest struct {
	Manifest

	Constructor func(meta map[string]string) (kvstore.DB, error)
}

type SyncSectorStateManifest struct {
	Manifest

	OnImport   func(args ...interface{}) error
	OnInit     func(args ...interface{}) error
	OnUpdate   func(args ...interface{}) error
	OnFinalize func(args ...interface{}) error
	OnRestore  func(args ...interface{}) error
}

type RegisterJsonRpcManifest struct {
	Manifest

	// Handler returns the jsonrpc namespace and handler
	// See: https://github.com/ipfs-force-community/go-jsonrpc/blob/4e8fb6324df7a31eaa6b480ef9e2a175545ba04b/server.go#L137
	Handler func() (namespace string, handler interface{})
}

// ExportManifest exports a manifest to VSM as a known format.
// it just casts sub-manifest to manifest.
func ExportManifest(m interface{}) *Manifest {
	v := reflect.ValueOf(m)
	return (*Manifest)(unsafe.Pointer(v.Pointer()))
}

// DeclareObjStoreManifest declares manifest as ObjStoreManifest.
func DeclareObjStoreManifest(m *Manifest) *ObjStoreManifest {
	return (*ObjStoreManifest)(unsafe.Pointer(m))
}

// DeclareKVStoreManifest declares manifest as KVStoreManifest.
func DeclareKVStoreManifest(m *Manifest) *KVStoreManifest {
	return (*KVStoreManifest)(unsafe.Pointer(m))
}

// DeclareSyncSectorStateManifest declares manifest as SyncSectorStateManifest.
func DeclareSyncSectorStateManifest(m *Manifest) *SyncSectorStateManifest {
	return (*SyncSectorStateManifest)(unsafe.Pointer(m))
}

// DeclareRegisterJsonRpcManifest declares manifest as DeclareRegisterJsonRpcManifest.
func DeclareRegisterJsonRpcManifest(m *Manifest) *RegisterJsonRpcManifest {
	return (*RegisterJsonRpcManifest)(unsafe.Pointer(m))
}
