package kubespy

import (
	"fmt"
	"log"
	"time"

	"github.com/fatih/color"
	"github.com/mbrlabs/uilive"
	"github.com/pulumi/kubespy/print"
	"github.com/pulumi/kubespy/watch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sWatch "k8s.io/apimachinery/pkg/watch"
)

const (
	v1Endpoints  = "v1/Endpoints"
	v1Service    = "v1/Service"
	v1Pod        = "v1/Pod"
	deployment   = "Deployment"
	v1ReplicaSet = "v1/ReplicaSet"
)

// Forked from
// https://github.com/pulumi/kubespy/blob/438edbfd5a9a72992803d45addb1f45b10a0b62f/cmd/trace.go
func TraceDeployment(namespace, name string) {
	// API server should rewrite this to apps/v1beta2, apps/v1beta2, or apps/v1 as appropriate.
	deploymentEvents, err := watch.Forever("apps/v1", "Deployment",
		watch.ThisObject(namespace, name))
	if err != nil {
		log.Fatal(err)
	}

	replicaSetEvents, err := watch.Forever("apps/v1", "ReplicaSet",
		watch.ObjectsOwnedBy(namespace, name))
	if err != nil {
		log.Fatal(err)
	}

	podEvents, err := watch.Forever("v1", "Pod", watch.All(namespace))
	if err != nil {
		log.Fatal(err)
	}

	writer := uilive.New()
	writer.RefreshInterval = time.Minute * 1
	writer.Start()      // Start listening for updates, render.
	defer writer.Stop() // Flush buffers, stop rendering.

	// Initial message.
	fmt.Fprintln(writer, color.New(color.FgCyan, color.Bold).Sprintf("Waiting for Deployment '%s/%s'",
		namespace, name))
	writer.Flush()

	table := map[string][]k8sWatch.Event{} // apiVersion/Kind -> []k8sWatch.Event
	repSets := map[string]k8sWatch.Event{} // Deployment name -> Pod
	pods := map[string]k8sWatch.Event{}    // ReplicaSet name -> Pod

	for {
		select {
		case e := <-deploymentEvents:
			if e.Type == k8sWatch.Deleted {
				o := e.Object.(*unstructured.Unstructured)
				delete(o.Object, "spec")
				delete(o.Object, "status")
			}
			table[deployment] = []k8sWatch.Event{e}
		case e := <-replicaSetEvents:
			o := e.Object.(*unstructured.Unstructured)
			if e.Type == k8sWatch.Deleted {
				delete(repSets, o.GetName())
			} else {
				repSets[o.GetName()] = e
			}
			table[v1ReplicaSet] = []k8sWatch.Event{}
			for _, rsEvent := range repSets {
				table[v1ReplicaSet] = append(table[v1ReplicaSet], rsEvent)
			}
		case e := <-podEvents:
			o := e.Object.(*unstructured.Unstructured)
			if e.Type == k8sWatch.Deleted {
				delete(pods, o.GetName())
			} else {
				pods[o.GetName()] = e
			}

			table[v1Pod] = []k8sWatch.Event{}
			for _, podEvent := range pods {
				table[v1Pod] = append(table[v1Pod], podEvent)
			}
		}
		print.DeploymentWatchTable(writer, table)
	}
}
