package vsmplugin

import (
	"context"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/ipfs-force-community/venus-cluster/vsm-plugin/kvstore"
	"github.com/ipfs-force-community/venus-cluster/vsm-plugin/objstore"
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
	Name        string
	Description string
	BuildTime   string
	// OnInit defines the plugin init logic.
	// it will be called after domain init.
	// return error will stop load plugin process and VSM startup.
	OnInit func(ctx context.Context, manifest *Manifest) error
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
