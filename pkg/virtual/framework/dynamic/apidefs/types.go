/*
Copyright 2022 The KCP Authors.

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

package apidefs

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/registry/rest"

	apiresourcev1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apiresource/v1alpha1"
)

// APIDomainKeyContextKeyType is the type of the key for the request context value
// that will carry the API domain key.
type APIDomainKeyContextKeyType string

// APIDomainKeyContextKey is the key for the request context value
// that will carry the API domain key.
const APIDomainKeyContextKey APIDomainKeyContextKeyType = "VirtualWorkspaceAPIDomainLocationKey"

// APIDefinition provides access to all the information needed to serve a given API resource
type APIDefinition interface {
	// GetAPIResourceSpec provides the API resource specification, which contains the
	// API names, sub-resource definitions, and the OpenAPIv3 schema.
	GetAPIResourceSpec() *apiresourcev1alpha1.CommonAPIResourceSpec

	// GetClusterName provides the name of the logical cluster where the resource specification comes from.
	GetClusterName() string

	// GetStorage provides the REST storage used to serve the resource.
	GetStorage() rest.Storage

	// GetSubResourceStorage provides the REST storage required to serve the given sub-resource.
	GetSubResourceStorage(subresource string) rest.Storage

	// GetRequestScope provides the handlers.RequestScope required to serve the resource.
	GetRequestScope() *handlers.RequestScope

	// GetSubResourceRequestScope provides the handlers.RequestScope required to serve the given sub-resource.
	GetSubResourceRequestScope(subresource string) *handlers.RequestScope
}

// APISet contains the APIDefintion objects for the APIs of an API domain.
type APISet map[schema.GroupVersionResource]APIDefinition

// APISetRetriever provides access to the API definitions of a API domain, based on the API domain key.
type APISetRetriever interface {
	GetAPIs(apiDomainKey string) (apis APISet, apisExist bool)
}

// CreateAPIDefinitionFunc is the type of a function which allows creating a complete APIDefinition
// (with REST storage and handler Request scopes) based on the API specification logical cluster name and OpenAPI v3 schema.
type CreateAPIDefinitionFunc func(logicalClusterName string, spec *apiresourcev1alpha1.CommonAPIResourceSpec) (APIDefinition, error)
