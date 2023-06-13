package objstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var httpLog = logging.New("objstore-http")

func ServeHTTP(ctx context.Context, mux *http.ServeMux, store Store) {
	instanceName := strings.Trim(store.Instance(ctx), "/")
	prefix := instanceName + "/"

	mux.Handle(prefix, http.StripPrefix(prefix, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		l := httpLog.With("path", r.URL.Path)
		var statusCode int
		var herr error

		defer func() {
			if herr != nil {
				if statusCode == 0 {
					statusCode = http.StatusInternalServerError
				}

				l.Warnw("got error", "code", statusCode, "err", herr)

				http.Error(rw, herr.Error(), statusCode)
				return
			}

			if statusCode != 0 {
				rw.WriteHeader(statusCode)
			}
		}()

		p := strings.TrimLeft(r.URL.Path, "/")
		if p == "" {
			statusCode = http.StatusBadRequest
			return
		}

		switch r.Method {
		case http.MethodGet:
			obj, err := store.Get(r.Context(), p)
			if err != nil {
				herr = fmt.Errorf("get object: %w", err)
				return
			}

			defer obj.Close()
			_, err = io.Copy(rw, obj)
			if err != nil {
				herr = fmt.Errorf("send object: %w", err)
				return
			}

		case http.MethodPut:
			_, err := store.Put(r.Context(), p, r.Body)
			if err != nil {
				herr = fmt.Errorf("put object: %w", err)
				return
			}

		case http.MethodHead:
			stat, err := store.Stat(r.Context(), p)
			if err != nil {
				if errors.Is(err, ErrObjectNotFound) {
					statusCode = http.StatusNotFound
					return
				}

				herr = fmt.Errorf("get stat of object: %w", err)
				return
			}

			rw.Header().Set("Content-Length", strconv.FormatInt(stat.Size, 10))

		default:
			statusCode = http.StatusMethodNotAllowed
			return
		}
	})))
}
