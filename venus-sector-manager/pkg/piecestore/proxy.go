package piecestore

import (
	"fmt"
	"io"
	"net/http"
	"strings"

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
		client: new (http.Client),
	}
}

type Proxy struct {
	locals []*filestore.Store
	market market.API
	client *http.Client
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(rw, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	path := strings.Trim(req.URL.Path, "/ ")
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

	redirectLocation := p.market.PieceResourceURL(c)
	rw.Header().Set("Content-Type", "application/octet-stream")
	http.Redirect(rw, req, redirectLocation, http.StatusNotFound)
}
