package plugable

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"plugin"
)

const SymbolNameConstructor = "Constructor"

// Constructor
type Constructor = func(name string, location string, meta map[string]string) (Plugable, error)

// For any object operations, fs.ErrNotExist should be returned if the object is not found
type Plugable interface {
	// type name of the plugable objstore
	Type() string

	// returns (free, total) volume space in bytes of the objstore instance
	Volume(ctx context.Context) (uint64, uint64, error)

	// returns the object reader
	Get(ctx context.Context, p string) (io.ReadCloser, error)

	// deletes the given object
	Del(ctx context.Context, p string) error

	// returns the stat of the given object
	Stat(ctx context.Context, p string) (fs.FileInfo, error)

	// put (override) an object to the store
	Put(ctx context.Context, p string, r io.Reader) (int64, error)

	// returns the full path of the given object, which may be used in other situations
	FullPath(ctx context.Context, p string) string
}

// loads the constructor from the given plugin
func LoadConstructor(path string) (Constructor, error) {
	plug, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plugin %s: %w", path, err)
	}

	sym, err := plug.Lookup(SymbolNameConstructor)
	if err != nil {
		return nil, fmt.Errorf("lookup symbol %s: %w", SymbolNameConstructor, err)
	}

	constructor, ok := sym.(Constructor)
	if !ok {
		return nil, fmt.Errorf("unexpected constructor object of type %T", sym)
	}

	return constructor, nil
}
