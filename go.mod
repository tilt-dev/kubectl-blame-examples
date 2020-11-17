module github.com/tilt-dev/kubectl-blame-examples

go 1.14

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0 // indirect
	github.com/fatih/color v1.9.0
	github.com/gofrs/flock v0.8.0 // indirect
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/lib/pq v1.8.0 // indirect
	github.com/mbrlabs/uilive v0.0.0-20170420192653-e481c8e66f15
	github.com/pkg/errors v0.9.1
	github.com/pulumi/kubespy v0.6.0
	github.com/pulumi/pulumi-kubernetes v1.6.0
	github.com/sirupsen/logrus v1.7.0 // indirect
	github.com/spf13/cobra v1.1.1
	github.com/tilt-dev/ctlptl v0.2.3-0.20201117045234-19005bb6afa6
	github.com/tilt-dev/localregistry-go v0.0.0-20201021185044-ffc4c827f097
	github.com/tjarratt/babble v0.0.0-20191209142150-eecdf8c2339d
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	helm.sh/helm/v3 v3.3.3
	k8s.io/api v0.19.2
	k8s.io/apiextensions-apiserver v0.18.8 // indirect
	k8s.io/apimachinery v0.19.2
	k8s.io/cli-runtime v0.19.2
	k8s.io/client-go v0.19.2
	k8s.io/klog v1.0.0 // indirect
	k8s.io/kubectl v0.18.8
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.1+incompatible
	github.com/evanphx/json-patch => github.com/evanphx/json-patch v0.0.0-20200808040245-162e5629780b // 162e5629780b is the SHA for git tag v4.8.0

	// workaround for
	// https://github.com/kubernetes/client-go/issues/741
	k8s.io/api => k8s.io/api v0.18.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.8
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.8
	k8s.io/client-go => k8s.io/client-go v0.18.8
	k8s.io/kubectl => k8s.io/kubectl v0.18.8
)
