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

package forwardingregistry

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// ClientGetter provides a way to get a dynamic client based on a given context.
// It is used to forward REST requests to a client that depends on the request context.
type ClientGetter interface {
	GetDynamicClient(ctx context.Context) (dynamic.Interface, error)
}

// ForwardingREST is a REST stofzage implementation that forwards the requests to a
// client-go dynamic client chosen based on the curret request context.
type ForwardingREST struct {
	// createStrategy allows extended behavior during creation, required
	createStrategy rest.RESTCreateStrategy
	// updateStrategy allows extended behavior during updates, required
	updateStrategy rest.RESTUpdateStrategy
	// resetFieldsStrategy provides the fields reset by the strategy that
	// should not be modified by the user.
	resetFieldsStrategy rest.ResetFieldsStrategy

	rest.TableConvertor

	resource schema.GroupVersionResource
	kind     schema.GroupVersionKind
	listKind schema.GroupVersionKind

	clientGetter ClientGetter
	subResources []string

	patchConflictRetryBackoff wait.Backoff
}

var _ rest.Lister = &ForwardingREST{}
var _ rest.Watcher = &ForwardingREST{}
var _ rest.Getter = &ForwardingREST{}
var _ rest.Updater = &ForwardingREST{}

// NewForwardingREST returns a REST storage that forwards calls to a dynamic client
func NewForwardingREST(resource schema.GroupVersionResource, kind, listKind schema.GroupVersionKind, strategy strategy, tableConvertor rest.TableConvertor, clientGetter ClientGetter, patchConflictRetryBackoff *wait.Backoff) (*ForwardingREST, *StatusREST) {
	if patchConflictRetryBackoff == nil {
		patchConflictRetryBackoff = &retry.DefaultRetry
	}
	mainREST := &ForwardingREST{
		createStrategy:      strategy,
		updateStrategy:      strategy,
		resetFieldsStrategy: strategy,

		TableConvertor: tableConvertor,

		resource: resource,
		kind:     kind,
		listKind: listKind,

		clientGetter: clientGetter,

		patchConflictRetryBackoff: *patchConflictRetryBackoff,
	}
	statusMainREST := *mainREST
	statusMainREST.subResources = []string{"status"}
	statusMainREST.updateStrategy = NewStatusStrategy(strategy)
	return mainREST,
		&StatusREST{
			mainREST: &statusMainREST,
		}
}

func (s *ForwardingREST) getClientResource(ctx context.Context) (dynamic.ResourceInterface, error) {
	client, err := s.clientGetter.GetDynamicClient(ctx)
	if err != nil {
		return nil, err
	}

	if s.createStrategy.NamespaceScoped() {
		if namespace, ok := genericapirequest.NamespaceFrom(ctx); ok {
			return client.Resource(s.resource).Namespace(namespace), nil
		} else {
			return nil, fmt.Errorf("there should be a Namespace context in a request for a namespaced resource: %s", s.resource.String())
		}
	} else {
		return client.Resource(s.resource), nil
	}
}

// New implements rest.Updater.
func (s *ForwardingREST) New() runtime.Object {
	ret := &unstructured.Unstructured{}
	ret.SetGroupVersionKind(s.kind)
	return ret
}

// NewList implements rest.Lister.
func (s *ForwardingREST) NewList() runtime.Object {
	ret := &unstructured.UnstructuredList{}
	ret.SetGroupVersionKind(s.listKind)
	return ret
}

// List implements rest.Lister.
func (s *ForwardingREST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	var v1ListOptions metav1.ListOptions
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &v1ListOptions, nil); err != nil {
		return nil, err
	}
	delegate, err := s.getClientResource(ctx)
	if err != nil {
		return nil, err
	}

	return delegate.List(ctx, v1ListOptions)
}

// Watch implements rest.Watcher.
func (s *ForwardingREST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	var v1ListOptions metav1.ListOptions
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &v1ListOptions, nil); err != nil {
		return nil, err
	}
	delegate, err := s.getClientResource(ctx)
	if err != nil {
		return nil, err
	}

	return delegate.Watch(ctx, v1ListOptions)
}

// Get implements rest.Getter
func (s *ForwardingREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	delegate, err := s.getClientResource(ctx)
	if err != nil {
		return nil, err
	}

	return delegate.Get(ctx, name, *options, s.subResources...)
}

// Update implements rest.Updater
func (s *ForwardingREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	delegate, err := s.getClientResource(ctx)
	if err != nil {
		return nil, false, err
	}

	doUpdate := func() (*unstructured.Unstructured, error) {
		oldObj, err := s.Get(ctx, name, &metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		obj, err := objInfo.UpdatedObject(ctx, oldObj)
		if err != nil {
			return nil, err
		}

		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return nil, fmt.Errorf("not an Unstructured: %#v", obj)
		}

		s.updateStrategy.PrepareForUpdate(ctx, obj, oldObj)
		if errs := s.updateStrategy.ValidateUpdate(ctx, obj, oldObj); len(errs) > 0 {
			return nil, kerrors.NewInvalid(unstructuredObj.GroupVersionKind().GroupKind(), unstructuredObj.GetName(), errs)
		}
		if err := updateValidation(ctx, obj.DeepCopyObject(), oldObj.DeepCopyObject()); err != nil {
			return nil, err
		}

		return delegate.Update(ctx, unstructuredObj, *options, s.subResources...)
	}

	requestInfo, _ := genericapirequest.RequestInfoFrom(ctx)
	if requestInfo != nil && requestInfo.Verb == "patch" {
		var result *unstructured.Unstructured
		err := retry.RetryOnConflict(s.patchConflictRetryBackoff, func() error {
			var err error
			result, err = doUpdate()
			return err
		})
		return result, false, err
	}

	result, err := doUpdate()
	return result, false, err
}

// GetResetFields implements rest.ResetFieldsStrategy
func (s *ForwardingREST) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	if s.resetFieldsStrategy == nil {
		return nil
	}
	return s.resetFieldsStrategy.GetResetFields()
}

func shallowCopyObjectMeta(u runtime.Unstructured) {
	obj := shallowMapDeepCopy(u.UnstructuredContent())
	if metadata, ok := obj["metadata"]; ok {
		if metadata, ok := metadata.(map[string]interface{}); ok {
			obj["metadata"] = shallowMapDeepCopy(metadata)
			u.SetUnstructuredContent(obj)
		}
	}
}

func shallowMapDeepCopy(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}

	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}

	return out
}

// StatusREST implements the REST endpoint for changing the status of a Resource
type StatusREST struct {
	mainREST *ForwardingREST
}

var _ = rest.Patcher(&StatusREST{})

func (r *StatusREST) New() runtime.Object {
	return r.mainREST.New()
}

// Get implements rest.Getter. It is required to support Patch.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	o, err := r.mainREST.Get(ctx, name, options)
	if err != nil {
		return nil, err
	}
	if u, ok := o.(*unstructured.Unstructured); ok {
		shallowCopyObjectMeta(u)
	}
	return o, nil
}

// Update alters the status subset of an object.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// We are explicitly setting forceAllowCreate to false in the call to the underlying storage because
	// subresources should never allow create on update.
	return r.mainREST.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}

// GetResetFields implements rest.ResetFieldsStrategy
func (r *StatusREST) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return r.mainREST.GetResetFields()
}
