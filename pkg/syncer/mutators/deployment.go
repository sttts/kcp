package mutators

import (
	"log"
	"net/url"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type DeploymentMutator struct {
	fromConfig          *rest.Config
	toConfig            *rest.Config
	registeredWorkspace string
}

func (dt *DeploymentMutator) getGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}
}

func (dt *DeploymentMutator) Register(mutators map[schema.GroupVersionResource]Mutator) {
	if _, ok := mutators[dt.getGVR()]; !ok {
		mutators[dt.getGVR()] = dt
	}
}

func NewDeploymentMutator(fromConfig, toConfig *rest.Config, registeredWorkspace string) *DeploymentMutator {
	return &DeploymentMutator{
		fromConfig:          fromConfig,
		toConfig:            toConfig,
		registeredWorkspace: registeredWorkspace,
	}
}

func (dt *DeploymentMutator) ApplyDownstreamName(downstreamObj *unstructured.Unstructured) error {
	// No transformations
	return nil
}

// ApplyStatus makes modifications to the Status of the deployment object.
func (dt *DeploymentMutator) ApplyStatus(upstreamObj *unstructured.Unstructured) error {
	// No transformations
	return nil
}

// ApplySpec makes modifications to the Spec of the deployment object.
func (dt *DeploymentMutator) ApplySpec(downstreamObj *unstructured.Unstructured) error {
	var deployment appsv1.Deployment
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		downstreamObj.UnstructuredContent(),
		&deployment)
	if err != nil {
		return err
	}

	// If the deployment has not serviceaccount defined or it is "default", that means that it is using the default one
	// and we need will need to override it with our own, "kcp-default"
	//
	// If the deployment has a serviceaccount defined, that means we need to synchronize the created service account
	// down to the workloadcluster, so we don't modify it, and expect the scheduler to do the job.
	//
	if deployment.Spec.Template.Spec.ServiceAccountName == "" || deployment.Spec.Template.Spec.ServiceAccountName == "default" {
		deployment.Spec.Template.Spec.ServiceAccountName = "kcp-default"
	}

	// Setting AutomountServiceAccountToken to false allow us to control the ServiceAccount
	// VolumeMount and Volume definitions.
	deployment.Spec.Template.Spec.AutomountServiceAccountToken = boolPtr(false)

	// In order to make the POD in the cluster be able to access the KCP api instead of the cluster api,
	// we will need to override the following env vars used by clients:
	//
	//  KUBERNETES_SERVICE_PORT=443
	//  KUBERNETES_SERVICE_PORT_HTTPS=443 <--- Not sure about this one
	//  KUBERNETES_SERVICE_HOST=10.96.0.1
	//
	//  And those too?:
	//
	//  KUBERNETES_PORT=tcp://10.96.0.1:443
	//  KUBERNETES_PORT_443_TCP_ADDR=10.96.0.1
	//  KUBERNETES_PORT_443_TCP_PORT=443
	//  KUBERNETES_PORT_443_TCP_PROTO=tcp
	//  KUBERNETES_PORT_443_TCP=tcp://10.96.0.1:443
	//
	// Where 10.96.0.1 is the IP of the default.kubernetes.svc.cluster.local, we will need to replace it
	// with the external address of the KCP api.

	// TODO(jmprusi): This is basically the KCP api address, but we need to add the workspace,
	//                and perhaps go through the shard proxy?
	u, err := url.Parse(dt.fromConfig.Host)
	if err != nil {
		log.Fatal(err)
	}

	kcpExternalPort := u.Port()
	kcpExternalHost := u.Hostname()

	// TODO(jmprusi): Is it safe to ALWAYS override those env vars? even when the deployment has some?
	overrideEnvs := []corev1.EnvVar{
		{Name: "KUBERNETES_SERVICE_PORT", Value: kcpExternalPort},
		{Name: "KUBERNETES_SERVICE_PORT_HTTPS", Value: kcpExternalPort},
		{Name: "KUBERNETES_SERVICE_HOST", Value: kcpExternalHost},
		// {Name: "KUBERNETES_PORT", Value: "tcp://" + kcpExternalHost + ":" + kcpExternalPort},
		// {Name: "KUBERNETES_PORT_443_TCP_ADDR", Value: kcpExternalHost},
		// {Name: "KUBERNETES_PORT_443_TCP_PORT", Value: kcpExternalPort},
		// {Name: "KUBERNETES_PORT_443_TCP_PROTO", Value: "tcp"},
		// {Name: "KUBERNETES_PORT_443_TCP", Value: "tcp://" + kcpExternalHost + ":" + kcpExternalPort},
	}

	// This is the VolumeMount that we will append to all the containers of the deployment
	serviceAccountMount := corev1.VolumeMount{
		Name:      "kcp-api-access",
		MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
		ReadOnly:  true,
	}

	// This is the Volume that we will add to the Deployment in order to control
	// the name of the ca.crt references (kcp-root-ca.crt vs kube-root-ca.crt)
	// and the serviceaccount reference.
	serviceAccountVolume := corev1.Volume{
		Name: "kcp-api-access",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				DefaultMode: int32ptr(420),
				Sources: []corev1.VolumeProjection{
					{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Path:              "token",
							ExpirationSeconds: int64ptr(3600),
						},
					},
					{
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "kcp-root-ca.crt",
							},
							Items: []corev1.KeyToPath{
								{
									Key:  "ca.crt",
									Path: "ca.crt",
								},
							},
						},
					},
					{
						DownwardAPI: &corev1.DownwardAPIProjection{
							Items: []corev1.DownwardAPIVolumeFile{
								{
									Path: "namespace",
									FieldRef: &corev1.ObjectFieldSelector{
										APIVersion: "v1",
										FieldPath:  "metadata.namespace",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// TODO(jmprusi): make sure those envs don't exist already.
	// Override Envs and add the VolumeMount to all the containers
	for i := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, overrideEnvs...)
		deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts, serviceAccountMount)
	}

	// Override Envs and add the VolumeMount to all the Init containers
	for i := range deployment.Spec.Template.Spec.InitContainers {
		deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, overrideEnvs...)
		deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts, serviceAccountMount)
	}

	// Override Envs and add the VolumeMount to all the Ephemeral containers
	for i := range deployment.Spec.Template.Spec.EphemeralContainers {
		deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, overrideEnvs...)
		deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts, serviceAccountMount)
	}

	// Add the ServiceAccount volume with our overrides.
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, serviceAccountVolume)

	unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
	if err != nil {
		return err
	}

	// Set the changes back into the obj.
	downstreamObj.SetUnstructuredContent(unstructured)

	return nil
}
func int64ptr(i int64) *int64 {
	return &i
}

func int32ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
