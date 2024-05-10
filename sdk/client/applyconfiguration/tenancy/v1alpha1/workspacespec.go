/*
Copyright The KCP Authors.

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

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha1

import (
	v3 "github.com/kcp-dev/logicalcluster/v3"
)

// WorkspaceSpecApplyConfiguration represents an declarative configuration of the WorkspaceSpec type for use
// with apply.
type WorkspaceSpecApplyConfiguration struct {
	Type     *WorkspaceTypeReferenceApplyConfiguration `json:"type,omitempty"`
	Location *WorkspaceLocationApplyConfiguration      `json:"location,omitempty"`
	Cluster  *v3.Name                                  `json:"cluster,omitempty"`
	URL      *string                                   `json:"URL,omitempty"`
}

// WorkspaceSpecApplyConfiguration constructs an declarative configuration of the WorkspaceSpec type for use with
// apply.
func WorkspaceSpec() *WorkspaceSpecApplyConfiguration {
	return &WorkspaceSpecApplyConfiguration{}
}

// WithType sets the Type field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Type field is set to the value of the last call.
func (b *WorkspaceSpecApplyConfiguration) WithType(value *WorkspaceTypeReferenceApplyConfiguration) *WorkspaceSpecApplyConfiguration {
	b.Type = value
	return b
}

// WithLocation sets the Location field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Location field is set to the value of the last call.
func (b *WorkspaceSpecApplyConfiguration) WithLocation(value *WorkspaceLocationApplyConfiguration) *WorkspaceSpecApplyConfiguration {
	b.Location = value
	return b
}

// WithCluster sets the Cluster field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Cluster field is set to the value of the last call.
func (b *WorkspaceSpecApplyConfiguration) WithCluster(value v3.Name) *WorkspaceSpecApplyConfiguration {
	b.Cluster = &value
	return b
}

// WithURL sets the URL field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the URL field is set to the value of the last call.
func (b *WorkspaceSpecApplyConfiguration) WithURL(value string) *WorkspaceSpecApplyConfiguration {
	b.URL = &value
	return b
}
