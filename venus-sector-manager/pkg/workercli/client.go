package workercli

import (
	"context"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

type Client struct {
	WorkerList   func(ctx context.Context) ([]core.WorkerThreadInfo, error)
	WorkerPause  func(ctx context.Context, index int) (bool, error)
	WorkerResume func(ctx context.Context, index int, setToState *string) (bool, error)
}

func Connect(ctx context.Context, endpoint string, opts ...jsonrpc.Option) (*Client, jsonrpc.ClientCloser, error) {
	var client Client
	closer, err := jsonrpc.NewMergeClient(ctx, endpoint, "VenusWorker", []interface{}{&client}, http.Header{}, opts...)
	return &client, closer, err
}
