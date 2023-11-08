package filestore

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

var httpLog = logging.New("filestore-http")

func ServeHTTP(ctx context.Context, mux *http.ServeMux, store Ext) {
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

		subPath := strings.TrimLeft(r.URL.Path, "/")
		if subPath == "" {
			statusCode = http.StatusBadRequest
			return
		}

		switch r.Method {
		case http.MethodGet:
			f, err := store.Read(r.Context(), subPath)
			if err != nil {
				herr = fmt.Errorf("get object: %w", err)
				return
			}

			defer f.Close()
			_, err = io.Copy(rw, f)
			if err != nil {
				herr = fmt.Errorf("send object: %w", err)
				return
			}

		case http.MethodPut:
			_, err := store.Write(r.Context(), subPath, r.Body)
			if err != nil {
				herr = fmt.Errorf("put object: %w", err)
				return
			}

		case http.MethodHead:
			stat, err := store.Stat(r.Context(), subPath)
			if err != nil {
				if errors.Is(err, ErrFileNotFound) {
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
