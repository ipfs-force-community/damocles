package piecestore

import (
	"fmt"
	"io"
	"net/http"

	"github.com/ipfs/go-cid"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
)

var log = logging.New("piecestore")

func NewProxy(locals []*filestore.Store, mapi market.API) *Proxy {
	return &Proxy{
		locals: locals,
		market: mapi,
	}
}

type Proxy struct {
	locals []*filestore.Store
	market market.API
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(rw, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	path := req.URL.Path
	c, err := cid.Decode(path)
	if err != nil {
		http.Error(rw, fmt.Sprintf("cast %s to cid: %s", path, err), http.StatusBadRequest)
		return
	}

	for _, store := range p.locals {
		if r, err := store.Get(req.Context(), path); err == nil {
			defer r.Close()
			_, err := io.Copy(rw, r)
			if err != nil {
				log.Warnw("transfer piece data for %s: %s", path, err)
			}
			return
		}
	}

	http.Redirect(rw, req, p.market.PieceResourceURL(c), http.StatusFound)
}
