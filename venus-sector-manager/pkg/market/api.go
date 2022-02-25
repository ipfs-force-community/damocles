package market

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/venus/venus-shared/api/market"
	mtypes "github.com/filecoin-project/venus/venus-shared/types/market"
	"github.com/ipfs/go-cid"
)

const (
	DealStatusUndefine = mtypes.Undefine
	DealStatusAssigned = mtypes.Assigned
	DealStatusPacking  = mtypes.Packing
	DealStatusProving  = mtypes.Proving
)

type API interface {
	market.IMarket

	PieceResourceURL(c cid.Cid) string
}

type (
	GetDealSpec         = mtypes.GetDealSpec
	DealInfoIncludePath = mtypes.DealInfoIncludePath
)

func New(ctx context.Context, addr, token string) (API, jsonrpc.ClientCloser, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid addr: %w", err)
	}

	if u.Scheme == "" || u.Host == "" {
		return nil, nil, fmt.Errorf("scheme or host is missing in addr")
	}

	// TODO: shoud be moved into venus-shared
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	cli, closer, err := market.NewIMarketRPC(ctx, addr, header)
	if err != nil {
		return nil, nil, fmt.Errorf("construct market api client: %w", err)
	}

	return &wrappedAPI{
		IMarket:          cli,
		resourceEndpoint: fmt.Sprintf("%s://%s/resource", u.Scheme, u.Host),
	}, closer, nil
}

type wrappedAPI struct {
	market.IMarket
	resourceEndpoint string
}

func (w *wrappedAPI) PieceResourceURL(c cid.Cid) string {
	return fmt.Sprintf("%s?resource-id=%s", w.resourceEndpoint, c.String())
}
