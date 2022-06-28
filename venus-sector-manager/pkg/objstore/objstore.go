package objstore

import (
	objstore "github.com/ipfs-force-community/venus-objstore"
)

// errors
var (
	ErrNotRegularFile              = objstore.ErrNotRegularFile
	ErrNotSeekable                 = objstore.ErrNotSeekable
	ErrNotOpened                   = objstore.ErrNotOpened
	ErrReadOnlyStore               = objstore.ErrReadOnlyStore
	ErrInvalidObjectPath           = objstore.ErrInvalidObjectPath
	ErrObjectStoreInstanceNotFound = objstore.ErrObjectStoreInstanceNotFound
	ErrObjectNotFound              = objstore.ErrObjectNotFound
)

// for store
var (
	DefaultConfig = objstore.DefaultConfig
)

type (
	CompactConfig = objstore.CompactConfig
	Config        = objstore.Config

	Stat         = objstore.Stat
	InstanceInfo = objstore.InstanceInfo
	Store        = objstore.Store
)

// for plugin
type Constructor = objstore.Constructor

var LoadConstructor = objstore.LoadConstructor
