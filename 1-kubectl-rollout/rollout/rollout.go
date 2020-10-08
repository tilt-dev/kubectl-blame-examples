package rollout

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	"k8s.io/kubectl/pkg/util/interrupt"
)

// Loosely adapted from
// https://github.com/kubernetes/kubectl/blob/5b27ac0ca2ba4fc3453941fcc23ebb54e35a099f/pkg/cmd/rollout/rollout_status.go
func WatchRollout(ctx context.Context, c dynamic.Interface, name string, revision int64) error {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", name).String()
	deployment := appsv1.SchemeGroupVersion.WithResource("deployments")
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return c.Resource(deployment).Namespace("default").List(context.TODO(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return c.Resource(deployment).Namespace("default").Watch(context.TODO(), options)
		},
	}

	// if the rollout isn't done yet, keep watching deployment status
	ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), 10*time.Second)
	intr := interrupt.New(nil, cancel)
	statusViewer := &polymorphichelpers.DeploymentStatusViewer{}
	return intr.Run(func() error {
		_, err := watchtools.UntilWithSync(ctx, lw, &unstructured.Unstructured{}, nil, func(e watch.Event) (bool, error) {
			switch t := e.Type; t {
			case watch.Added, watch.Modified:
				status, done, err := statusViewer.Status(e.Object.(runtime.Unstructured), revision)
				if err != nil {
					return false, err
				}
				fmt.Printf("%s", status)
				// Quit waiting if the rollout is done
				if done {
					return true, nil
				}

				return false, nil

			case watch.Deleted:
				// We need to abort to avoid cases of recreation and not to silently watch the wrong (new) object
				return true, fmt.Errorf("object has been deleted")

			default:
				return true, fmt.Errorf("internal error: unexpected event %#v", e)
			}
		})
		return err
	})
}
