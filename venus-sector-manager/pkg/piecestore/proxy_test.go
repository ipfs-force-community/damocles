package piecestore

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
	"github.com/jbenet/go-random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/venus/venus-shared/api/market/mock"
)

func setupStoreProxy(t *testing.T, resourceEndPoint string) *Proxy {
	sts, err := filestore.OpenStores([]objstore.Config{
		{
			Name:     "mock test",
			Path:     os.TempDir(),
			Strict:   false,
			ReadOnly: false,
		},
	})

	require.NoError(t, err, "open mock store")

	mc := gomock.NewController(t)
	marketAPI := &market.WrappedAPI{
		IMarket:          mock.NewMockIMarket(mc),
		ResourceEndpoint: resourceEndPoint,
	}
	return NewProxy(sts, marketAPI)
}

func TestStorePoxy(t *testing.T) {
	ctx := context.Background()

	t.Run("download for common piece store", func(t *testing.T) {
		storeProxy := setupStoreProxy(t, "mock")
		resourceID := "bafy2bzacea2a75bbdhr6gjozglrmp4akkgzsw3xh3s62j3hga5uhmkxwne5b6"
		tmpFile, err := os.CreateTemp(os.TempDir(), "piece_proxy")
		require.NoError(t, err)
		require.NoError(t, random.WriteRandomBytes(100, tmpFile))
		tmpFile.Seek(0, io.SeekStart) //nolint
		expectBytes, err := ioutil.ReadAll(tmpFile)
		require.NoError(t, err)
		tmpFile.Seek(0, io.SeekStart) //nolint
		_, err = storeProxy.locals[0].Put(ctx, resourceID, tmpFile)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())

		path := fmt.Sprintf("http://127.0.0.1:3030/%s", resourceID)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		storeProxy.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		result, err := ioutil.ReadAll(w.Body)
		assert.Nil(t, err)
		assert.Equal(t, expectBytes, result)
	})

	t.Run("download from market server", func(t *testing.T) {
		resourceID := "bafy2bzacecc4iu4nsmm5vqkj427xtkqjedcclo77glct2j5rhrrohe3xj7zpw"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()

		storeProxy := setupStoreProxy(t, srv.URL)
		path := fmt.Sprintf("http://127.0.0.1:3030/%s", resourceID)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		storeProxy.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		if val, ok := w.Header()["Location"]; ok {
			assert.Equal(t, fmt.Sprintf("%s?resource-id=%s", srv.URL, resourceID), val[0])
		} else {
			assert.FailNow(t, "expect redirect header but not found")
		}
	})
}
