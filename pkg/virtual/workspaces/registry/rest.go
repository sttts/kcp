package registry

import (
	"context"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"
)

type REST struct {
	// Allows extended behavior during creation, required
	createStrategy rest.RESTCreateStrategy
	// Allows extended behavior during updates, required
	updateStrategy rest.RESTUpdateStrategy

	rest.TableConvertor
}

var _ rest.Lister = &REST{}
var _ rest.Scoper = &REST{}

// NewREST returns a RESTStorage object that will work against Workspace resources
func NewREST() *REST {
	return &REST{
		createStrategy: Strategy,
		updateStrategy: Strategy,

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator()},
	}
}

// New returns a new Project
func (s *REST) New() runtime.Object {
	return &tenancyv1alpha1.Workspace{}
}

// NewList returns a new ProjectList
func (*REST) NewList() runtime.Object {
	return &tenancyv1alpha1.WorkspaceList{}
}

func (s *REST) NamespaceScoped() bool {
	return false
}

// List retrieves a list of Projects that match label.

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
/*
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, kerrors.NewForbidden(tenancyv1alpha1.Resource("workspace"), "", fmt.Errorf("unable to list workspaces without a user on the context"))
	}
	labelSelector, _ := InternalListOptionsToSelectors(options)
*/
	return &tenancyv1alpha1.WorkspaceList{
		Items: []tenancyv1alpha1.Workspace{
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: tenancyv1alpha1.SchemeGroupVersion.Identifier(),
					Kind: "Workspace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "hello",
					ClusterName: "aClusterName",			
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{			
				},
				Status: tenancyv1alpha1.WorkspaceStatus{
					BaseURL: "https://kjkjh:6443/khkh",
				},
			},			
		},
	}, nil
}

var _ = rest.Getter(&REST{})

// Get retrieves a Project by name
func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return &tenancyv1alpha1.Workspace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: tenancyv1alpha1.SchemeGroupVersion.Identifier(),
			Kind: "Workspace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello",
			ClusterName: "aClusterName",			
		},
		Spec: tenancyv1alpha1.WorkspaceSpec{			
		},
		Status: tenancyv1alpha1.WorkspaceStatus{
			BaseURL: "https://kjkjh:6443/khkh",
		},
	}, nil
}

