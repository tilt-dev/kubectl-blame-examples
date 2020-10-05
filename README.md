# kubecon-2020-talk

[![Build Status](https://circleci.com/gh/tilt-dev/kubecon-2020-talk/tree/master.svg?style=shield)](https://circleci.com/gh/tilt-dev/kubecon-2020-talk)

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
4) Start a port-forwarding server at `localhost:8000` when the deployment is finished

The main difference between each project is how they track the deployment.

## 0-naive

Creates a random image tag and label for each deployment. Watches that label.

## 1-kubectl-rollout

Uses the approach of `kubectl rollout`, waiting for the deployment to report success

## 2-helm-wait

Uses the approach of `helm --wait`, looking up the replicaset and waiting for it to report success

## 3-kubespy-trace

Uses the approach of `kubespy trace`, using owner references to find everything.

## 4-tilt-now

Uses the current Tilt approach, with a combination of owner refs and template hashes.

## License

Copyright 2020 Windmill Engineering

Licensed under [the Apache License, Version 2.0](LICENSE)
