package piecestore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/market"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore/filestore"
	"github.com/jbenet/go-random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/venus/venus-shared/api/market/v1/mock"
)

var (
	FALSE = false
	ONE   = uint(1)
)

func setupStoreProxy(t *testing.T, resourceEndPoint string) *Proxy {
	st, err := filestore.Open(objstore.Config{
		Name: "mock test",
		Path: os.TempDir(),
	}, false)

	require.NoError(t, err, "open mock store")

	mc := gomock.NewController(t)
	marketAPI := &market.WrappedAPI{
		IMarket:          mock.NewMockIMarket(mc),
		ResourceEndpoint: resourceEndPoint,
	}
	return NewProxy([]objstore.Store{st}, marketAPI)
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
		expectBytes, err := io.ReadAll(tmpFile)
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
		result, err := io.ReadAll(w.Body)
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

func TestStoreRead(t *testing.T) {
	storePath := os.TempDir()
	st, err := filestore.Open(objstore.Config{
		Name: "mock test",
		Path: storePath,
	}, false)
	require.NoError(t, err, "open mock store")

	ctx := context.Background()
	resourceID := "bafy2bzacecc4iu4nsmm5vqkj427xtkqjedcclo77glct2j5rhrrohe3xj7zpw"
	tmpFile, err := os.CreateTemp(os.TempDir(), "piece_proxy")
	require.NoError(t, err)
	require.NoError(t, random.WriteRandomBytes(100, tmpFile))
	tmpFile.Seek(0, io.SeekStart) //nolint
	expectBytes, err := io.ReadAll(tmpFile)
	require.NoError(t, err)
	tmpFile.Seek(0, io.SeekStart) //nolint
	_, err = st.Put(ctx, resourceID, tmpFile)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	t.Run("read from local store", func(t *testing.T) {
		r, err := st.Get(ctx, resourceID)
		require.NoError(t, err)
		result, err := io.ReadAll(r)
		assert.Nil(t, err)
		assert.Equal(t, expectBytes, result)
	})
}
