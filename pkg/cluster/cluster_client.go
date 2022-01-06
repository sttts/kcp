package cluster

import (
	"net/http"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

type httpClient struct {
	delegate rest.HTTPClient
}

func NewHTTPClient(delegate rest.HTTPClient) *httpClient {
	return &httpClient{
		delegate: delegate,
	}
}

func (c *httpClient) Do(req *http.Request) (*http.Response, error) {
	clusterName, err := FromContext(req.Context())
	if err != nil {
		// Couldn't find cluster name in context
		return c.delegate.Do(req)
	}

	if !strings.HasPrefix(req.URL.Path, "/clusters/") {
		originalPath := req.URL.Path

		// start with /clusters/$name
		req.URL.Path = "/clusters/" + clusterName

		// if the original path is relative, add a / separator
		if len(originalPath) > 0 && originalPath[0] != '/' {
			req.URL.Path += "/"
		}

		// finally append the original path
		req.URL.Path += originalPath
	}

	return c.delegate.Do(req)
}

func (c *httpClient) Timeout() time.Duration {
	return c.delegate.Timeout()
}

func (c *httpClient) Transport() http.RoundTripper {
	return c.delegate.Transport()
}
