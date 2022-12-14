module github.com/tilt-dev/kubectl-blame-examples

go 1.14

require (
	github.com/fatih/color v1.13.0
	github.com/mbrlabs/uilive v0.0.0-20170420192653-e481c8e66f15
	github.com/pkg/errors v0.9.1
	github.com/pulumi/kubespy v0.6.0
	github.com/tilt-dev/ctlptl v0.2.3-0.20201117045234-19005bb6afa6
	github.com/tjarratt/babble v0.0.0-20191209142150-eecdf8c2339d
	helm.sh/helm/v3 v3.10.3
	k8s.io/api v0.25.2
	k8s.io/apimachinery v0.25.2
	k8s.io/cli-runtime v0.25.2
	k8s.io/client-go v0.25.2
	k8s.io/kubectl v0.25.2
	sigs.k8s.io/yaml v1.3.0
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
