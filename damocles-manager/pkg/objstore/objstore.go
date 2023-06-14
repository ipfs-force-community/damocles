package objstore

import (
	"github.com/ipfs-force-community/damocles/manager-plugin/objstore"
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
	Config = objstore.Config

	Stat         = objstore.Stat
	InstanceInfo = objstore.InstanceInfo
	Store        = objstore.Store
)
