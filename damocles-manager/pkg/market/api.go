package market

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/venus/venus-shared/api"

	mkapi "github.com/filecoin-project/venus/venus-shared/api/market/v1"
	mtypes "github.com/filecoin-project/venus/venus-shared/types/market"
)

const (
	DealStatusUndefine = mtypes.Undefine
	DealStatusAssigned = mtypes.Assigned
	DealStatusPacking  = mtypes.Packing
	DealStatusProving  = mtypes.Proving
)

type API interface {
	mkapi.IMarket

	PieceResourceURL(c cid.Cid) string
}

type (
	GetDealSpec         = mtypes.GetDealSpec
	DealInfoIncludePath = mtypes.DealInfoIncludePath
)

func New(ctx context.Context, addr, token string) (API, jsonrpc.ClientCloser, error) {
	ainfo := api.NewAPIInfo(addr, token)

	dialAddr, err := ainfo.DialArgs(api.VerString(mkapi.MajorVersion))
	if err != nil {
		return nil, nil, fmt.Errorf("get dial args for connecting: %w", err)
	}

	cli, closer, err := mkapi.NewIMarketRPC(ctx, dialAddr, ainfo.AuthHeader(), jsonrpc.WithRetry(true))
	if err != nil {
		return nil, nil, fmt.Errorf("construct market api client: %w", err)
	}

	u, err := url.Parse(dialAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("%q is not a valid url: %w", dialAddr, err)
	}

	switch u.Scheme {
	case "ws":
		u.Scheme = "http"

	case "wss":
		u.Scheme = "https"
	}

	return &WrappedAPI{
		IMarket:          cli,
		ResourceEndpoint: fmt.Sprintf("%s://%s/resource", u.Scheme, u.Host),
	}, closer, nil
}

type WrappedAPI struct {
	mkapi.IMarket
	ResourceEndpoint string
}

func (w *WrappedAPI) PieceResourceURL(c cid.Cid) string {
	return fmt.Sprintf("%s?resource-id=%s", w.ResourceEndpoint, c.String())
}
