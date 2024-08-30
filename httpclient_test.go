package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dummyClient struct {
	statusCode int
	body       string
	requests   int
}

func (d *dummyClient) Do(req *http.Request) (*http.Response, error) {
	d.requests++
	return &http.Response{
		StatusCode: d.statusCode,
		Body:       io.NopCloser(strings.NewReader(d.body)),
	}, nil
}

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("cache miss, hit, miss", func(t *testing.T) {
		dummyClient := &dummyClient{
			statusCode: http.StatusOK,
			body:       "hello, world",
		}
		store := NewStore(t.TempDir(), WithClient(dummyClient), WithMaxErrorVersion(2))

		req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
		require.NoError(t, err)

		rsp, found, storeFunc, err := store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.False(t, found)
		data, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 1, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.True(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 1, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Now())
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 2, dummyClient.requests)
	})

	t.Run("two different endpoints", func(t *testing.T) {
		dummyClient := &dummyClient{
			statusCode: http.StatusOK,
			body:       "hello, world",
		}
		store := NewStore(t.TempDir(), WithClient(dummyClient), WithMaxErrorVersion(2))

		req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
		require.NoError(t, err)

		rsp, found, storeFunc, err := store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.False(t, found)
		data, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 1, dummyClient.requests)

		req, err = http.NewRequest(http.MethodGet, "http://example2.com", nil)
		require.NoError(t, err)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 2, dummyClient.requests)
	})

	t.Run("fetch error retry max", func(t *testing.T) {
		dummyClient := &dummyClient{
			statusCode: http.StatusInternalServerError,
			body:       "hello, world",
		}
		store := NewStore(t.TempDir(), WithClient(dummyClient), WithMaxErrorVersion(2))

		req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
		require.NoError(t, err)

		rsp, found, storeFunc, err := store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.False(t, found)
		data, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 1, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 2, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 3, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.True(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 3, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Now())
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 4, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 5, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.False(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 6, dummyClient.requests)

		rsp, found, storeFunc, err = store.CacheFetch(req, time.Time{})
		require.NoError(t, storeFunc())
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rsp.StatusCode)
		require.True(t, found)
		data, err = io.ReadAll(rsp.Body)
		require.NoError(t, err)
		require.Equal(t, "hello, world", string(data))
		require.Equal(t, 6, dummyClient.requests)
	})
}
