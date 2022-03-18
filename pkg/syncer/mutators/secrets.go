package mutators

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type SecretMutator struct {
	fromConfig          *rest.Config
	toConfig            *rest.Config
	registeredWorkspace string
}

func (sm *SecretMutator) getGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}
}

func (sm *SecretMutator) Register(mutators map[schema.GroupVersionResource]Mutator) {
	if _, ok := mutators[sm.getGVR()]; !ok {
		mutators[sm.getGVR()] = sm
	}
}

func NewSecretMutator(fromConfig, toConfig *rest.Config, registeredWorkspace string) *SecretMutator {
	return &SecretMutator{
		fromConfig:          fromConfig,
		toConfig:            toConfig,
		registeredWorkspace: registeredWorkspace,
	}
}

func (sm *SecretMutator) ApplyDownstreamName(downstreamObj *unstructured.Unstructured) error {
	// No transformations
	return nil
}

// ApplyStatus makes modifications to the Status of the deployment object.
func (sm *SecretMutator) ApplyStatus(upstreamObj *unstructured.Unstructured) error {
	// No transformations
	return nil
}

// ApplySpec makes modifications to the Spec of the deployment object.
func (sm *SecretMutator) ApplySpec(downstreamObj *unstructured.Unstructured) error {
	var secret corev1.Secret
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		downstreamObj.UnstructuredContent(),
		&secret)
	if err != nil {
		return err
	}

	secret.Type = corev1.SecretTypeOpaque
	secret.Annotations["kubernetes.io/service-account.name"] = "kcp-default"
	secret.Annotations["kubernetes.io/service-account.uid"] = ""

	unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secret)
	if err != nil {
		return err
	}

	// Set the changes back into the obj.
	downstreamObj.SetUnstructuredContent(unstructured)

	return nil
}
