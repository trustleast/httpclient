# httpclient

[![Go build status](https://github.com/trustleast/httpclient/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/trustleast/httpclient/actions/workflows/go.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/trustleast/httpclient)](https://goreportcard.com/report/github.com/trustleast/httpclient) [![Go Reference](https://pkg.go.dev/badge/github.com/trustleast/httpclient.svg)](https://pkg.go.dev/github.com/trustleast/httpclient)

httpclient is a package that makes it easy to repeatedly fetch resources from an HTTP server.
It provides a wrapper over the stdlib http.Client Do function.
It stores raw responses on the file system with a timestamp to indicate when it was fetched.
Using this, you can use modified times to trigger refetches or not.

The files are stored gzipped on disk to save space.

It also provides convenience functions to determine whether an element is fetched from cache or not.

## Usage

Drop in replacement (NOTE: This will always fetch from local cache)
```go
package main

import (
    "fmt"
    "net/url"

    "github.com/trustleast/httpclient"
)

func main() {
    store := httpclient.NewStore("/tmp")
    req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)

    // Fetch from remote
    rsp, _ := store.Do(r)
    data1 := io.ReadAll(rsp.Body)
    fmt.Println(string(data1))

    // Fetch from local
    rsp, _ = store.Do(r)
    data2 := io.ReadAll(rsp.Body)
    fmt.Println(string(data2)
}
```

Only fetch from remote every 5 minutes
```go
package main

import (
    "fmt"
    "net/url"

    "github.com/trustleast/httpclient"
)

func main() {
    store := httpclient.NewStore("/tmp")
    req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)

    // Fetch from remote
    rsp, _ := store.Do(r)
    data1 := io.ReadAll(rsp.Body)
    fmt.Println(string(data1))

    // Fetch from local
    rsp, cached, _ := store.CacheFetchAndStore(r, time.Now().Add(-5 * time.Minute))
    data2 := io.ReadAll(rsp.Body)
    fmt.Println(string(data2)
    fmt.Println(cached) // true

    time.Sleep(5 * time.Minute)

    // Fetch from remote
    rsp, cached, _ = store.CacheFetchAndStore(r, time.Now().Add(-5 * time.Minute))
    data3 := io.ReadAll(rsp.Body)
    fmt.Println(string(data3))
    fmt.Println(cached) // false
}
```
