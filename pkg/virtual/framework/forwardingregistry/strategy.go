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

	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	structurallisttype "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/listtype"
	schemaobjectmeta "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/objectmeta"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kube-openapi/pkg/validation/validate"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
)

var typerSchema = runtime.NewScheme()

func init() {
	_ = tenancyv1alpha1.AddToScheme(typerSchema)
}

func NewStrategy(typer runtime.ObjectTyper, namespaceScoped bool, kind schema.GroupVersionKind, schemaValidator, statusSchemaValidator *validate.SchemaValidator, structuralSchema *structuralschema.Structural, statusEnabled bool) strategy {
	return strategy{
		ObjectTyper:     typer,
		NameGenerator:   names.SimpleNameGenerator,
		namespaceScoped: namespaceScoped,
		statusEnabled:   statusEnabled,
		validator: validator{
			namespaceScoped:       namespaceScoped,
			kind:                  kind,
			schemaValidator:       schemaValidator,
			statusSchemaValidator: statusSchemaValidator,
		},
		structuralSchema: structuralSchema,
		kind:             kind,
	}
}

// strategy implements behavior for resources served by the ForwadingRest REST storage.
type strategy struct {
	runtime.ObjectTyper
	names.NameGenerator

	namespaceScoped  bool
	statusEnabled    bool
	kind             schema.GroupVersionKind
	validator        validator
	structuralSchema *structuralschema.Structural
}

func (s strategy) NamespaceScoped() bool {
	return s.namespaceScoped
}

// GetResetFields returns the set of fields that get reset by the strategy
// and should not be modified by the user.
func (s strategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	fields := map[fieldpath.APIVersion]*fieldpath.Set{}

	if s.statusEnabled {
		fields[fieldpath.APIVersion(s.kind.GroupVersion().String())] = fieldpath.NewSet(
			fieldpath.MakePathOrDie("status"),
		)
	}

	return fields
}

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (s strategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if s.statusEnabled {
		customResourceObject := obj.(*unstructured.Unstructured)
		customResource := customResourceObject.UnstructuredContent()

		// create cannot set status
		delete(customResource, "status")
	}

	accessor, _ := meta.Accessor(obj)
	accessor.SetGeneration(1)
}

// PrepareForUpdate clears fields that are not allowed to be set by end users on update.
func (s strategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newCustomResourceObject := obj.(*unstructured.Unstructured)
	oldCustomResourceObject := old.(*unstructured.Unstructured)

	newCustomResource := newCustomResourceObject.UnstructuredContent()
	oldCustomResource := oldCustomResourceObject.UnstructuredContent()

	// If the /status subresource endpoint is installed, update is not allowed to set status.
	if s.statusEnabled {
		_, ok1 := newCustomResource["status"]
		_, ok2 := oldCustomResource["status"]
		switch {
		case ok2:
			newCustomResource["status"] = oldCustomResource["status"]
		case ok1:
			delete(newCustomResource, "status")
		}
	}

	// except for the changes to `metadata`, any other changes
	// cause the generation to increment.
	newCopyContent := copyNonMetadata(newCustomResource)
	oldCopyContent := copyNonMetadata(oldCustomResource)
	if !apiequality.Semantic.DeepEqual(newCopyContent, oldCopyContent) {
		oldAccessor, _ := meta.Accessor(oldCustomResourceObject)
		newAccessor, _ := meta.Accessor(newCustomResourceObject)
		newAccessor.SetGeneration(oldAccessor.GetGeneration() + 1)
	}
}

func copyNonMetadata(original map[string]interface{}) map[string]interface{} {
	ret := make(map[string]interface{})
	for key, val := range original {
		if key == "metadata" {
			continue
		}
		ret[key] = val
	}
	return ret
}

// Validate validates a new workspace.
func (s strategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, s.validator.Validate(ctx, obj)...)

	// validate embedded resources
	if u, ok := obj.(*unstructured.Unstructured); ok {
		errs = append(errs, schemaobjectmeta.Validate(nil, u.Object, s.structuralSchema, false)...)

		// validate x-kubernetes-list-type "map" and "set" invariant
		errs = append(errs, structurallisttype.ValidateListSetsAndMaps(nil, s.structuralSchema, u.Object)...)
	}

	return errs
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (strategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

// AllowCreateOnUpdate is false for ForwardingRest resources; this means a POST is
// needed to create one.
func (strategy) AllowCreateOnUpdate() bool {
	return false
}

// AllowUnconditionalUpdate is the default update policy for ForwardingRest resource objects.
func (strategy) AllowUnconditionalUpdate() bool {
	return false
}

// Canonicalize normalizes the object after validation
func (strategy) Canonicalize(obj runtime.Object) {
}

// ValidateUpdate is the default update validation for an end user.
func (s strategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, s.validator.ValidateUpdate(ctx, obj, old)...)

	uNew, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return errs
	}
	uOld, ok := old.(*unstructured.Unstructured)
	if !ok {
		return errs
	}

	// Checks the embedded objects. We don't make a difference between update and create for those.
	errs = append(errs, schemaobjectmeta.Validate(nil, uNew.Object, s.structuralSchema, false)...)

	// ratcheting validation of x-kubernetes-list-type value map and set
	if oldErrs := structurallisttype.ValidateListSetsAndMaps(nil, s.structuralSchema, uOld.Object); len(oldErrs) == 0 {
		errs = append(errs, structurallisttype.ValidateListSetsAndMaps(nil, s.structuralSchema, uNew.Object)...)
	}

	return errs
}

// WarningsOnUpdate returns warnings for the given update.
func (strategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func (a strategy) GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, nil, err
	}
	return labels.Set(accessor.GetLabels()), objectMetaFieldsSet(accessor, a.namespaceScoped), nil
}

// objectMetaFieldsSet returns a fields that represent the ObjectMeta.
func objectMetaFieldsSet(objectMeta metav1.Object, namespaceScoped bool) fields.Set {
	if namespaceScoped {
		return fields.Set{
			"metadata.name":      objectMeta.GetName(),
			"metadata.namespace": objectMeta.GetNamespace(),
		}
	}
	return fields.Set{
		"metadata.name": objectMeta.GetName(),
	}
}
