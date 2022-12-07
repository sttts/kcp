module github.com/kcp-dev/kcp/pkg/apis

go 1.18

require (
	github.com/google/go-cmp v0.5.8
	github.com/kcp-dev/logicalcluster/v3 v3.0.0-20221206202856-4d025e96f4a8
	github.com/onsi/gomega v1.10.1
	github.com/stretchr/testify v1.7.1
	k8s.io/api v0.24.3
	k8s.io/apiextensions-apiserver v0.24.3
	k8s.io/apimachinery v0.24.3
	k8s.io/klog/v2 v2.70.1
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v0.0.0-20210826220005-b48c857c3a0e // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/cel-go v0.10.1 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/genproto v0.0.0-20220107163113-42d7afdf6368 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/kube-openapi v0.0.0-20220328201542-3ee0da9b0b42 // indirect
	k8s.io/utils v0.0.0-20220728103510-ee6ede2d64ed // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	github.com/kcp-dev/apimachinery => ../../../apimachinery
	github.com/kcp-dev/client-go => ../../../client-go
	github.com/kcp-dev/kcp/pkg/apis => ./pkg/apis
	github.com/kcp-dev/logicalcluster/v3 => ../../../logicalcluster
	k8s.io/api => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/api
	k8s.io/apiextensions-apiserver => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver
	k8s.io/apimachinery => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/apimachinery
	k8s.io/apiserver => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/apiserver
	k8s.io/cli-runtime => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/cli-runtime
	k8s.io/client-go => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/client-go
	k8s.io/cloud-provider => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/cloud-provider
	k8s.io/cluster-bootstrap => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap
	k8s.io/code-generator => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/code-generator
	k8s.io/component-base => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/component-base
	k8s.io/component-helpers => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/component-helpers
	k8s.io/controller-manager => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/controller-manager
	k8s.io/cri-api => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/cri-api
	k8s.io/csi-translation-lib => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib
	k8s.io/kube-aggregator => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator
	k8s.io/kube-controller-manager => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager
	k8s.io/kube-proxy => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/kube-proxy
	k8s.io/kube-scheduler => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler
	k8s.io/kubectl => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/kubectl
	k8s.io/kubelet => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/kubelet
	k8s.io/kubernetes => ../../../../../k8s.io/kubernetes
	k8s.io/legacy-cloud-providers => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers
	k8s.io/metrics => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/metrics
	k8s.io/mount-utils => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/mount-utils
	k8s.io/pod-security-admission => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/pod-security-admission
	k8s.io/sample-apiserver => ../../../../../k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver
)
