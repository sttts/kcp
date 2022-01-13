/*
Copyright 2021 The KCP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/genericcontrolplane"

	"github.com/kcp-dev/kcp/pkg/controllerz"
)

var reClusterName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,78}[a-z0-9]$`)

func ServeHTTP(apiHandler http.Handler) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		var clusterName string
		// klog.Infof("ANDY incoming request: %s", req.URL.String())
		if path := req.URL.Path; strings.HasPrefix(path, "/clusters/") {
			path = strings.TrimPrefix(path, "/clusters/")
			i := strings.Index(path, "/")
			if i == -1 {
				responsewriters.ErrorNegotiated(
					errors.NewBadRequest(fmt.Sprintf("unable to parse cluster: no `/` found in path %s", path)),
					scheme.Codecs, schema.GroupVersion{},
					w, req)
				return
			}
			clusterName, path = path[:i], path[i:]
			req.URL.Path = path
			for i := 0; i < 2 && len(req.URL.RawPath) > 1; i++ {
				slash := strings.Index(req.URL.RawPath[1:], "/")
				if slash == -1 {
					responsewriters.ErrorNegotiated(
						errors.NewInternalError(fmt.Errorf("unable to parse cluster when shortening raw path, have clusterName=%q, rawPath=%q", clusterName, req.URL.RawPath)),
						scheme.Codecs, schema.GroupVersion{},
						w, req)
					return
				}
				req.URL.RawPath = req.URL.RawPath[slash:]
			}
		} else {
			clusterName = req.Header.Get("X-Kubernetes-Cluster")
		}
		var cluster genericapirequest.Cluster
		switch clusterName {
		case "*":
			// HACK: just a workaround for testing
			cluster.Wildcard = true
		case "":
			clusterName = genericcontrolplane.RootClusterName
		default:
			if !reClusterName.MatchString(clusterName) {
				responsewriters.ErrorNegotiated(
					errors.NewBadRequest(fmt.Sprintf("invalid cluster: %q does not match the regex", clusterName)),
					scheme.Codecs, schema.GroupVersion{},
					w, req)
				return
			}
		}
		cluster.Name = clusterName
		ctx := req.Context()
		//ctx = genericapirequest.WithCluster(req.Context(), cluster)
		scope := controllerz.NewScope(clusterName, controllerz.WildcardScope(cluster.Wildcard))
		// Set the scope for caches/listers
		ctx = rest.WithScope(ctx, scope)
		// Set the scope for storage handlers
		ctx = storage.WithScope(ctx, scope)
		apiHandler.ServeHTTP(w, req.WithContext(ctx))
	}
}
