package httpclient

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	_timestampKey = "X-Elucidate-Time"
	_versionKey   = "X-Elucidate-Version"

	// ETag keys
	_etagKey        = "Etag"
	_ifNoneMatchKey = "If-None-Match"

	_minimumContentLength = 10
	_defaultAllowedErrors = 3
)

type (
	Client interface {
		Do(r *http.Request) (resp *http.Response, err error)
	}

	Store struct {
		fileSystemRoot string

		client            Client
		maxErrorVersion   int
		fetchTimestampKey string
		fetchVersionKey   string

		seenHosts sync.Map
	}

	StoreFunc func() error
)

type Option func(*Store)

func WithClient(client Client) Option {
	return func(s *Store) {
		s.client = client
	}
}

func WithMaxErrorVersion(maxErrorVersion int) Option {
	return func(s *Store) {
		s.maxErrorVersion = maxErrorVersion
	}
}

func WithFetchTimestampKey(key string) Option {
	return func(s *Store) {
		s.fetchTimestampKey = key
	}
}

func WithFetchVersionKey(key string) Option {
	return func(s *Store) {
		s.fetchVersionKey = key
	}
}

func NewStore(fileSystemRoot string, opts ...Option) *Store {
	s := &Store{
		fileSystemRoot:    fileSystemRoot,
		client:            http.DefaultClient,
		maxErrorVersion:   _defaultAllowedErrors,
		fetchTimestampKey: _timestampKey,
		fetchVersionKey:   _versionKey,
		seenHosts:         sync.Map{},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// CacheFetch tries to fetch the request from cache and falls back to remote if validations fail.
// It returns a boolean indicating whether it received it from the cache or not.
// It also returns a StoreFunc that should be called if the client wants to store the response in cache.
func (s *Store) CacheFetch(ctx context.Context, r *http.Request, lastModified time.Time) (*http.Response, bool, StoreFunc, error) {
	r = r.WithContext(ctx)
	rsp, cached, storeFunc, err := s.internalCacheFetch(r, lastModified)
	return rsp, cached, storeFunc, err
}

// CacheFetchAndStore is like CacheFetch but it immediately calls the store function after getting a response.
func (s *Store) CacheFetchAndStore(ctx context.Context, r *http.Request, lastModified time.Time) (*http.Response, bool, error) {
	rsp, cached, storeFunc, err := s.CacheFetch(ctx, r, lastModified)
	if err != nil {
		return rsp, cached, err
	}

	if err := storeFunc(); err != nil {
		return rsp, cached, err
	}

	return rsp, cached, nil
}

// Do is a convenience method to adhere to the HTTPClient interface (for easy interoperability with existing http calls.
// It sets the lastModified time to 0, so it will always fetch from cache.
// Be careful using this as you will never get updated results.
func (s *Store) Do(r *http.Request) (*http.Response, error) {
	rsp, _, storeFunc, err := s.CacheFetch(r.Context(), r, time.Time{})
	if err != nil {
		return rsp, err
	}

	if err := storeFunc(); err != nil {
		return rsp, err
	}

	return rsp, nil
}

func (s *Store) RawCacheData(u *url.URL) ([]byte, error) {
	key := fsKey(s.fileSystemRoot, u)
	gzipReader, err := readGZIPFile(key)

	if err != err {
		return nil, err
	}

	data, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, err
	}
	if err := gzipReader.Close(); err != nil {
		return nil, err
	}

	return data, nil
}

func (s *Store) internalCacheFetch(r *http.Request, lastModified time.Time) (*http.Response, bool, StoreFunc, error) {
	key := fsKey(s.fileSystemRoot, r.URL)

	// TODO: We should probably cache this so we don't do it on every request
	hostDir := filepath.Dir(key)
	if _, ok := s.seenHosts.Load(hostDir); !ok {
		if err := os.MkdirAll(hostDir, 0750); err != nil {
			return nil, false, NoOpStoreFunc, err
		}
		s.seenHosts.Store(hostDir, struct{}{})
	}

	version := 0
	etag := ""
	cachedRsp, err := readAndParseGZIPFile(key, r)
	if err == nil {
		storedVersion, ok := s.shouldUseCachedValue(cachedRsp, lastModified)
		if ok {
			return cachedRsp, true, NoOpStoreFunc, nil
		}

		etag = cachedRsp.Header.Get(_etagKey)
		version = storedVersion
	}

	r.Header = r.Header.Clone()
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.Header.Set(_ifNoneMatchKey, etag)
	rsp, err := s.client.Do(r)
	if err != nil {
		return rsp, false, NoOpStoreFunc, err
	}

	if rsp.StatusCode == http.StatusNotModified {
		// Should we write back to cache here?
		return cachedRsp, true, NoOpStoreFunc, nil
	}

	finalStoreFunc, err := s.prepareForWriting(key, version+1, rsp)
	if err != nil {
		return nil, false, NoOpStoreFunc, fmt.Errorf("failed to write fixture: %w", err)
	}

	return rsp, false, finalStoreFunc, nil
}

func (s *Store) shouldUseCachedValue(rsp *http.Response, lastModified time.Time) (int, bool) {
	if rsp.ContentLength != -1 && rsp.ContentLength < _minimumContentLength {
		return 0, false
	}

	t, err := s.getWriteTimestamp(rsp)
	if err != nil {
		return 0, false
	}
	version, err := s.getVersion(rsp)
	if err != nil {
		return 0, false
	}

	switch rsp.StatusCode {
	case http.StatusForbidden, http.StatusUnauthorized, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		if version <= s.maxErrorVersion {
			return version, false
		}
	}

	if t.After(lastModified) {
		return version, true
	}

	return 0, false
}

func (s *Store) getWriteTimestamp(rsp *http.Response) (time.Time, error) {
	if timestamp := rsp.Header.Get(s.fetchTimestampKey); timestamp != "" {
		seconds, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		t := time.Unix(seconds, 0)
		return t, nil
	}
	return time.Time{}, fmt.Errorf("failed to find timestamp")
}

func (s *Store) getVersion(rsp *http.Response) (int, error) {
	if version := rsp.Header.Get(s.fetchVersionKey); version != "" {
		return strconv.Atoi(version)
	}
	return 1, nil
}

func (s *Store) prepareForWriting(key string, version int, rsp *http.Response) (func() error, error) {
	if rsp.Header == nil {
		rsp.Header = make(http.Header)
	}
	rsp.Header.Set(s.fetchTimestampKey, fmt.Sprintf("%d", time.Now().Unix()))
	rsp.Header.Set(s.fetchVersionKey, fmt.Sprintf("%d", version))

	dump, err := httputil.DumpResponse(rsp, true)
	if err != nil {
		return nil, fmt.Errorf("failed to dump response: %w", err)
	}

	return func() error {
		var buffer bytes.Buffer
		gzipWriter := gzip.NewWriter(&buffer)
		_, err = gzipWriter.Write(dump)
		if err != nil {
			return err
		}

		if err := gzipWriter.Close(); err != nil {
			return err
		}

		return os.WriteFile(key, buffer.Bytes(), 0640)
	}, nil
}

func readGZIPFile(key string) (*gzip.Reader, error) {
	f, err := os.ReadFile(key)
	if err != nil {
		return nil, err
	}

	return gzip.NewReader(bytes.NewReader(f))
}

func readAndParseGZIPFile(key string, r *http.Request) (*http.Response, error) {
	gzipReader, err := readGZIPFile(key)
	if err != nil {
		return nil, err
	}

	return http.ReadResponse(bufio.NewReader(gzipReader), r)
}

func fsKey(fileSystemRoot string, u *url.URL) string {
	cleaned_params := strings.ReplaceAll(u.RawQuery, "/", "-")
	noLeadingSlash := strings.TrimLeft(u.Path, "/")
	cleaned_path := strings.ReplaceAll(noLeadingSlash, "/", "-")
	return filepath.Join(fileSystemRoot, u.Host, strings.ToLower(cleaned_path+"?"+cleaned_params+".gz"))
}

func NoOpStoreFunc() error {
	return nil
}
