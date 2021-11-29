package registry

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
)

// workspaceStrategy implements behavior for workspaces
type workspaceStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// Strategy is the default logic that applies when creating and updating Project
// objects via the REST API.
var Strategy = workspaceStrategy{legacyscheme.Scheme, names.SimpleNameGenerator}

// NamespaceScoped is false for workspaces.
func (workspaceStrategy) NamespaceScoped() bool {
	return false
}

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (workspaceStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
}

// PrepareForUpdate clears fields that are not allowed to be set by end users on update.
func (workspaceStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
}

// Validate validates a new workspace.
func (workspaceStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return nil
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (workspaceStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

// AllowCreateOnUpdate is false for project.
func (workspaceStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (workspaceStrategy) AllowUnconditionalUpdate() bool {
	return false
}

// Canonicalize normalizes the object after validation.
func (workspaceStrategy) Canonicalize(obj runtime.Object) {
}

// ValidateUpdate is the default update validation for an end user.
func (workspaceStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return nil
}

// WarningsOnUpdate returns warnings for the given update.
func (workspaceStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}
