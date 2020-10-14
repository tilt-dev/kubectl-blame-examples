# kubectl-blame-examples

[![Build Status](https://circleci.com/gh/tilt-dev/kubectl-blame-examples/tree/main.svg?style=shield)](https://circleci.com/gh/tilt-dev/kubectl-blame-examples)

This repository contains a simplified version of Tilt that's used for teaching and talks.

[In Search of a `kubectl blame` Command](https://sched.co/ekAv) at [KubeCon 2020](https://events.linuxfoundation.org/kubecon-cloudnativecon-north-america/)

Each subdirectory contains a sample app that you can run with:

```
cd $DIR
go run ./main.go
```

Each sample app does the same basic flow:

1) Build and push the image (with `docker build`, `docker push`, and [some magic](https://github.com/tilt-dev/localregistry-go) to detect the registry)
2) Apply the deployment (with `kubectl apply`)
3) Track the deployment's progress (with `kubernetes/client-go`)

The main difference between each project is how they track the deployment.

## [0-naive](0-naive)

Creates a random image tag and label for each deployment. Watches that label.

**Code:** [main.go](0-naive/main.go)

## [1-kubectl-rollout](1-kubectl-rollout)

Uses the approach of `kubectl rollout`, waiting for the deployment to report success.

**Code:** 
- [main.go](1-kubectl-rollout/main.go)
- [rollout.go](1-kubectl-rollout/rollout/rollout.go) forked from [rollout_status.go](https://github.com/kubernetes/kubectl/blob/5b27ac0ca2ba4fc3453941fcc23ebb54e35a099f/pkg/cmd/rollout/rollout_status.go)

## [2-helm](2-helm)

Uses the approach of `helm --wait`, looking up the replicaset and waiting for it to report success

**Code:**
- [main.go](2-helm/main.go)
- Uses the Helm Kube client off the shelf, which is fun to read! [wait.go](https://github.com/helm/helm/blob/fc9b46067f8f24a90b52eba31e09b31e69011e93/pkg/kube/wait.go#L52)

## [3-kubespy](3-kubespy)

Uses the approach of `kubespy trace`, using owner references to find everything.

**Code:**
- [main.go](3-kubespy/main.go)
- [trace.go](3-kubespy/kubespy/trace.go) forked from `traceDeployment` in [trace.go](https://github.com/pulumi/kubespy/blob/438edbfd5a9a72992803d45addb1f45b10a0b62f/cmd/trace.go#L104)

## [4-tilt](4-tilt)

Uses the current Tilt approach, with a combination of owner refs and template hashes.

**Code:**
- [main.go](4-tilt/main.go)
- [pod_template_hash.go](4-tilt/tilt/pod_template_hash.go) computes labels, forked from [pod_template.go](https://github.com/tilt-dev/tilt/blob/9511b7fdf7ca171d8094ff3b5828df8dfa2dd64d/internal/k8s/pod_template.go)
- [owner_fetcher_go.go](4-tilt/tilt/owner_fetcher.go) computes the owner tree, forked from [owner_fetcher.go](https://github.com/tilt-dev/tilt/blob/9511b7fdf7ca171d8094ff3b5828df8dfa2dd64d/internal/k8s/owner_fetcher.go)

## License

Copyright 2020 Windmill Engineering

Licensed under [the Apache License, Version 2.0](LICENSE)
