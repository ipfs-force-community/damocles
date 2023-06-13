package workercli

import (
	"context"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

// no context to avoid span info
type Client struct {
	WorkerList   func() ([]core.WorkerThreadInfo, error)
	WorkerPause  func(index uint64) (bool, error)
	WorkerResume func(index uint64, setToState *string) (bool, error)
}

func Connect(ctx context.Context, endpoint string, opts ...jsonrpc.Option) (*Client, jsonrpc.ClientCloser, error) {
	var client Client
	closer, err := jsonrpc.NewMergeClient(ctx, endpoint, "VenusWorker", []interface{}{&client}, http.Header{}, opts...)
	return &client, closer, err
}
