package chain

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/pkg/constants"
	"github.com/filecoin-project/venus/venus-shared/api"
	v1 "github.com/filecoin-project/venus/venus-shared/api/chain/v1"
	"github.com/filecoin-project/venus/venus-shared/types"
)

const (
	HCRevert  = types.HCRevert
	HCApply   = types.HCApply
	HCCurrent = types.HCCurrent
)

type (
	HeadChange = types.HeadChange
	Partition  = types.Partition
)

type API = v1.FullNode

func New(ctx context.Context, api, token string) (API, jsonrpc.ClientCloser, error) {
	client, closer, err := DialFullNodeRPC(ctx, api, token, nil, jsonrpc.WithRetry(true))
	if err != nil {
		return nil, nil, err
	}

	client.IChainInfoStruct.Internal.StateNetworkVersion = cacheStateNetworkVersion(client.IChainInfoStruct.Internal.StateNetworkVersion)

	return client, closer, nil
}

type MockStruct = v1.FullNodeStruct

type stateNetworkVersion func(p0 context.Context, p1 types.TipSetKey) (network.Version, error)

// cacheStateNetworkVersion will cache the network version for a block delay.
// You should be careful to use this function, make sure the params passed in is the same as the one to be replaced
// And it can lead to latency of chain version up to a block delay
func cacheStateNetworkVersion(inner stateNetworkVersion) stateNetworkVersion {
	nv := constants.TestNetworkVersion - 1
	var latestUpdate time.Time

	return func(p0 context.Context, p1 types.TipSetKey) (network.Version, error) {
		if nv < constants.TestNetworkVersion && time.Since(latestUpdate) > time.Second*time.Duration(constants.MainNetBlockDelaySecs) {
			var err error
			nv, err = inner(p0, p1)
			if err != nil {
				return nv, err
			}
			latestUpdate = time.Now()
			return nv, nil
		}
		return nv, nil
	}
}

// DialFullNodeRPC is modify from v1.DialFullNodeRPC, which will return a FullNodeStruct directly rather then a FullNode interface
func DialFullNodeRPC(ctx context.Context, addr string, token string, requestHeader http.Header, opts ...jsonrpc.Option) (*v1.FullNodeStruct, jsonrpc.ClientCloser, error) {
	ainfo := api.NewAPIInfo(addr, token)
	endpoint, err := ainfo.DialArgs(api.VerString(v1.MajorVersion))
	if err != nil {
		return nil, nil, fmt.Errorf("get dial args: %w", err)
	}

	if requestHeader == nil {
		requestHeader = http.Header{}
	}
	requestHeader.Set(api.VenusAPINamespaceHeader, v1.APINamespace)
	ainfo.SetAuthHeader(requestHeader)

	var res v1.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, endpoint, v1.MethodNamespace, api.GetInternalStructs(&res), requestHeader, opts...)

	return &res, closer, err
}
