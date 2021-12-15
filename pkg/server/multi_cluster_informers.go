package server

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	tenancyapi "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	kcpexternalversions "github.com/kcp-dev/kcp/pkg/client/informers/externalversions"
)

// type genericInformer interface {
// 	Informer() cache.SharedIndexInformer
// 	Lister() cache.GenericLister
// }

type genericInformerFactory interface {
	ForResource(resource schema.GroupVersionResource) (kcpexternalversions.GenericInformer, error)
}

func Foo(scheme *runtime.Scheme, mapper meta.RESTMapper, factory genericInformerFactory) {
	knownTypes := scheme.AllKnownTypes()
	for _, typ := range knownTypes {
		typeName := typ.Name()
		fmt.Printf("Evaluating %q\n", typeName)
		v := reflect.New(typ)
		i := v.Interface()
		ro, ok := i.(runtime.Object)
		if !ok {
			panic(fmt.Errorf("Not a runtime.Object: %T", i))
		}
		objectKinds, unversioned, err := scheme.ObjectKinds(ro)
		if err != nil {
			panic(err)
		}
		if unversioned {
			fmt.Println("unversioned")
		}
		for _, k := range objectKinds {
			fmt.Printf("Got GVK: %v\n", k.String())
			mapping, err := mapper.RESTMapping(k.GroupKind(), tenancyapi.SchemeGroupVersion.Version)
			if err != nil {
				if meta.IsNoMatchError(err) {
					continue
				}
				panic(err)
			}
			fmt.Printf("KIND: %v\nRESOURCE: %v\n", k.GroupKind(), mapping.Resource)
			informer, err := factory.ForResource(mapping.Resource)
			if err != nil {
				fmt.Printf("Error getting informer: %v\n", err)
				continue
			}
			if err := AddMultiClusterIndexes(informer.Informer()); err != nil {
				fmt.Printf("Error adding indexes: %v\n", err)
			}
			fmt.Printf("SUCCESS for %v\n", mapping.Resource)

		}
	}
	fmt.Println("ANDY DONE")
}

const LogicalClusterIndex = "lcluster"

func AddMultiClusterIndexes(informer cache.SharedIndexInformer) error {
	return informer.AddIndexers(cache.Indexers{
		LogicalClusterIndex: func(obj interface{}) ([]string, error) {
			metaObj, ok := obj.(metav1.Object)
			if !ok {
				return []string{}, nil
			}

			return []string{metaObj.GetClusterName()}, nil
		},
	})
}
