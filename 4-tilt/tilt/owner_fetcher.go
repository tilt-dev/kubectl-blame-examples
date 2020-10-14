package tilt

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// The ObjectRefTree only contains immutable properties
// of a Kubernetes object: the name, namespace, and UID
type ObjectRefTree struct {
	Ref    v1.ObjectReference
	Owners []ObjectRefTree
}

func (t ObjectRefTree) ContainsUID(uid types.UID) bool {
	if t.Ref.UID == uid {
		return true
	}
	for _, owner := range t.Owners {
		if owner.ContainsUID(uid) {
			return true
		}
	}
	return false
}

func (t ObjectRefTree) stringLines() []string {
	result := []string{fmt.Sprintf("%s:%s", t.Ref.Kind, t.Ref.Name)}
	for _, owner := range t.Owners {
		// indent each of the owners by two spaces
		branchLines := owner.stringLines()
		for _, branchLine := range branchLines {
			result = append(result, fmt.Sprintf("  %s", branchLine))
		}
	}
	return result
}

func (t ObjectRefTree) String() string {
	return strings.Join(t.stringLines(), "\n")
}

type restMapper interface {
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

type resourceNamespace struct {
	Namespace string
	GVK       schema.GroupVersionKind
}

type OwnerFetcher struct {
	globalCtx  context.Context
	restMapper restMapper
	metadata   metadata.Interface
	cache      map[types.UID]*objectTreePromise
	mu         *sync.Mutex

	metaCache       map[types.UID]*metav1.ObjectMeta
	resourceFetches map[resourceNamespace]*sync.Once
}

func NewOwnerFetcher(ctx context.Context, config *rest.Config) OwnerFetcher {
	meta, err := metadata.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		panic(err)
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))

	return OwnerFetcher{
		globalCtx:  ctx,
		restMapper: mapper,
		metadata:   meta,
		cache:      make(map[types.UID]*objectTreePromise),
		mu:         &sync.Mutex{},

		metaCache:       make(map[types.UID]*metav1.ObjectMeta),
		resourceFetches: make(map[resourceNamespace]*sync.Once),
	}
}

func (v OwnerFetcher) getOrCreateResourceFetch(gvk schema.GroupVersionKind, ns string) *sync.Once {
	v.mu.Lock()
	defer v.mu.Unlock()
	rns := resourceNamespace{Namespace: ns, GVK: gvk}
	fetch, ok := v.resourceFetches[rns]
	if !ok {
		fetch = &sync.Once{}
		v.resourceFetches[rns] = fetch
	}
	return fetch
}

// As an optimization, we batch fetch all the ObjectMetas of a resource type
// the first time we need that resource, then watch updates.
func (v OwnerFetcher) ensureResourceFetched(gvk schema.GroupVersionKind, ns string) {
	fetch := v.getOrCreateResourceFetch(gvk, ns)
	fetch.Do(func() {
		mapping, err := v.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			log.Printf("Error mapping GroupVersionKind: %v", err)
			return
		}
		gvr := mapping.Resource

		metas, err := v.metadata.Resource(gvr).Namespace(ns).List(v.globalCtx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Error fetching metadata: %v", err)
			return
		}

		v.mu.Lock()
		for _, meta := range metas.Items {
			v.metaCache[meta.ObjectMeta.GetUID()] = &meta.ObjectMeta
		}
		v.mu.Unlock()

		w, err := v.metadata.Resource(gvr).Namespace(ns).Watch(v.globalCtx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Error watching metadata: %v", err)
			return
		}

		go func() {
			for event := range w.ResultChan() {
				m, ok := event.Object.(*metav1.PartialObjectMetadata)
				if ok {
					v.mu.Lock()
					v.metaCache[m.ObjectMeta.GetUID()] = &m.ObjectMeta
					v.mu.Unlock()
				}
			}
		}()
	})
}

// Returns a promise and a boolean. The boolean is true if the promise is
// already in progress, and false if the caller is responsible for
// resolving/rejecting the promise.
func (v OwnerFetcher) getOrCreatePromise(id types.UID) (*objectTreePromise, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	promise, ok := v.cache[id]
	if !ok {
		promise = newObjectTreePromise()
		v.cache[id] = promise
	}
	return promise, ok
}

func (v OwnerFetcher) OwnerTreeOfRef(ctx context.Context, ref v1.ObjectReference) (result ObjectRefTree, err error) {
	uid := ref.UID
	if uid == "" {
		return ObjectRefTree{}, fmt.Errorf("Can only get owners of deployed entities")
	}

	promise, ok := v.getOrCreatePromise(uid)
	if ok {
		return promise.wait()
	}

	defer func() {
		if err != nil {
			promise.reject(err)
		} else {
			promise.resolve(result)
		}
	}()

	meta, err := v.getMetaByReference(ctx, ref)
	if err != nil {
		if errors.IsNotFound(err) {
			return ObjectRefTree{Ref: ref}, nil
		}
		return ObjectRefTree{}, err
	}
	return v.ownerTreeOfHelper(ctx, ref, meta)
}

func (v OwnerFetcher) getMetaByReference(ctx context.Context, ref v1.ObjectReference) (*metav1.ObjectMeta, error) {
	gvk := ref.GroupVersionKind()
	mapping, err := v.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	gvr := mapping.Resource
	v.ensureResourceFetched(gvk, string(ref.Namespace))

	v.mu.Lock()
	meta, ok := v.metaCache[ref.UID]
	v.mu.Unlock()

	if ok {
		return meta, nil
	}

	obj, err := v.metadata.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return &obj.ObjectMeta, nil
}

func (v OwnerFetcher) OwnerTreeOf(ctx context.Context, obj runtime.Object) (result ObjectRefTree, err error) {
	t := reflect.ValueOf(obj).Elem().FieldByName("TypeMeta").Interface().(metav1.TypeMeta)
	meta := reflect.ValueOf(obj).Elem().FieldByName("ObjectMeta").Interface().(metav1.ObjectMeta)
	uid := meta.GetUID()
	if uid == "" {
		return ObjectRefTree{}, fmt.Errorf("Can only get owners of deployed entities")
	}

	promise, ok := v.getOrCreatePromise(uid)
	if ok {
		return promise.wait()
	}

	defer func() {
		if err != nil {
			promise.reject(err)
		} else {
			promise.resolve(result)
		}
	}()

	ref := v1.ObjectReference{
		Name:       meta.Name,
		Namespace:  meta.Namespace,
		Kind:       t.Kind,
		UID:        meta.UID,
		APIVersion: t.APIVersion,
	}
	return v.ownerTreeOfHelper(ctx, ref, &meta)
}

func (v OwnerFetcher) ownerTreeOfHelper(ctx context.Context, ref v1.ObjectReference, meta *metav1.ObjectMeta) (ObjectRefTree, error) {
	tree := ObjectRefTree{Ref: ref}
	owners := meta.GetOwnerReferences()
	for _, owner := range owners {
		ownerRef := v1.ObjectReference{
			Name:       owner.Name,
			Namespace:  meta.GetNamespace(),
			Kind:       owner.Kind,
			UID:        owner.UID,
			APIVersion: owner.APIVersion,
		}
		ownerTree, err := v.OwnerTreeOfRef(ctx, ownerRef)
		if err != nil {
			return ObjectRefTree{}, err
		}
		tree.Owners = append(tree.Owners, ownerTree)
	}
	return tree, nil
}

type objectTreePromise struct {
	tree ObjectRefTree
	err  error
	done chan struct{}
}

func newObjectTreePromise() *objectTreePromise {
	return &objectTreePromise{
		done: make(chan struct{}),
	}
}

func (e *objectTreePromise) resolve(tree ObjectRefTree) {
	e.tree = tree
	close(e.done)
}

func (e *objectTreePromise) reject(err error) {
	e.err = err
	close(e.done)
}

func (e *objectTreePromise) wait() (ObjectRefTree, error) {
	<-e.done
	return e.tree, e.err
}
