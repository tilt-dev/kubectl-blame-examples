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

## 0-naive

Creates a random image tag and label for each deployment. Watches that label.

**Code:** [main.go](0-naive/main.go)

## 1-kubectl-rollout

Uses the approach of `kubectl rollout`, waiting for the deployment to report success.

**Code:** 
- [main.go](1-kubectl-rollout/main.go)
- [rollout.go](1-kubectl-rollout/rollout/rollout.go) forked from [rollout_status.go](https://github.com/kubernetes/kubectl/blob/master/pkg/cmd/rollout/rollout_status.go)

## 2-helm-wait

Uses the approach of `helm --wait`, looking up the replicaset and waiting for it to report success

## 3-kubespy-trace

Uses the approach of `kubespy trace`, using owner references to find everything.

## 4-tilt-now

Uses the current Tilt approach, with a combination of owner refs and template hashes.

## License

Copyright 2020 Windmill Engineering

Licensed under [the Apache License, Version 2.0](LICENSE)
