package mutators

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type ServiceAccountMutator struct {
	fromConfig          *rest.Config
	toConfig            *rest.Config
	registeredWorkspace string
}

func (sm *ServiceAccountMutator) getGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "serviceaccounts",
	}
}

func (sm *ServiceAccountMutator) Register(mutators map[schema.GroupVersionResource]Mutator) {
	if _, ok := mutators[sm.getGVR()]; !ok {
		mutators[sm.getGVR()] = sm
	}
}

func NewServiceAccountMutator(fromConfig, toConfig *rest.Config, registeredWorkspace string) *ServiceAccountMutator {
	return &ServiceAccountMutator{
		fromConfig:          fromConfig,
		toConfig:            toConfig,
		registeredWorkspace: registeredWorkspace,
	}
}

func (sm *ServiceAccountMutator) ApplyDownstreamName(downstreamObj *unstructured.Unstructured) error {
	// No transformations
	return nil
}

// ApplyStatus makes modifications to the Status of the deployment object.
func (sm *ServiceAccountMutator) ApplyStatus(upstreamObj *unstructured.Unstructured) error {
	// No transformations
	return nil
}

// ApplySpec makes modifications to the Spec of the deployment object.
func (sm *ServiceAccountMutator) ApplySpec(downstreamObj *unstructured.Unstructured) error {
	var serviceaccount corev1.ServiceAccount
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		downstreamObj.UnstructuredContent(),
		&serviceaccount)
	if err != nil {
		return err
	}

	for i, _ := range serviceaccount.Secrets {
		if strings.Contains(serviceaccount.Secrets[i].Name, "default-token") {
			//randomString := strings.Split(serviceaccount.Secrets[i].Name, "-")
			serviceaccount.Secrets[i].Name = "kcp-default-token" //-" + randomString[len(randomString)-1]
		}
	}

	unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&serviceaccount)
	if err != nil {
		return err
	}

	// Set the changes back into the obj.
	downstreamObj.SetUnstructuredContent(unstructured)

	return nil
}
