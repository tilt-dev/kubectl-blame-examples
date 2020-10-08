package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/tilt-dev/localregistry-go"
	"github.com/tjarratt/babble"
	"helm.sh/helm/v3/pkg/kube"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	yamlEncoder "sigs.k8s.io/yaml"
)

var alphaRegexp = regexp.MustCompile("[^a-zA-Z-]")

func main() {
	var seed int64
	var contentName string
	var crash bool
	flag.Int64Var(&seed, "seed", time.Now().UnixNano(), "Seed the random label generator")
	flag.StringVar(&contentName, "contents", "", "Contents of index.html. Defaults to the random label")
	flag.BoolVar(&crash, "crash", false, "When set, replaces the entrypoint on the container so it crashes")
	flag.Parse()
	rand.Seed(seed)

	// Generate a random label for this deployment
	id := sanitize(babble.NewBabbler().Babble())
	if contentName == "" {
		contentName = id
	}
	contents := fmt.Sprintf("Hello world! I'm deployment %s!", contentName)
	imageTag := fmt.Sprintf("deploy-%x", md5.Sum([]byte(contentName)))

	c := client()
	imageRef := generateImageRef(c, imageTag)

	// Generate the contents of index.html
	fmt.Printf("Generated index.html = `%s`\n", contents)
	contentsTarball := tarball(contents)

	// Build + push
	_ = cmd(fmt.Sprintf("docker build -t %s -", imageRef),
		withStdin(contentsTarball))

	_ = cmd(fmt.Sprintf("docker push %s", imageRef))

	// Modify the Deployment and apply
	deployment := appsv1.Deployment{}
	decodeFile("./deployment.yaml", &deployment)

	deployment.Spec.Template.Spec.Containers[0].Image = imageRef

	if crash {
		fmt.Println(`[go] Adding command = ["exit", "1"] because --crash=true`)
		deployment.Spec.Template.Spec.Containers[0].Command = []string{"exit", "1"}
	}

	_ = cmd("kubectl apply -o yaml -f -", withStdin(encode(deployment)))

	helmKubeClient := kube.New(nil)
	helmKubeClient.Log = func(f string, args ...interface{}) {
		fmt.Println(fmt.Sprintf(f, args...))
	}

	res := &resource.Info{
		Namespace: "default",
		Name:      deployment.Name,
		Object:    &deployment,
	}
	resList := []*resource.Info{res}

	color.Green(fmt.Sprintf("[go] helm wait %s\n", deployment.Name))
	err := helmKubeClient.Wait(resList, 15*time.Second)
	if err != nil {
		panic(err)
	}
	color.Green("deployed successfully")
}

func sanitize(s string) string {
	return strings.ToLower(alphaRegexp.ReplaceAllString(s, ""))
}

func generateImageRef(c *kubernetes.Clientset, tag string) string {
	registry, _ := localregistry.Discover(context.Background(), c.CoreV1())
	if registry.Host == "" {
		fmt.Println("This script requires a cluster with a discoverable registry.\n" +
			"See https://github.com/tilt-dev/kind-local for an example of how to set this up with Kind")
		os.Exit(1)
	}

	imageName := path.Join(registry.Host, "my-busybox")
	return fmt.Sprintf("%s:%s", imageName, tag)
}

func prettyAge(pod *v1.Pod) string {
	t := pod.CreationTimestamp.Time
	dur := time.Since(t)
	return fmt.Sprintf("%.3fs", float64(dur)/float64(time.Second))
}

func tarball(contents string) io.Reader {
	b := bytes.NewBuffer(nil)
	w := tar.NewWriter(b)
	dockerfile := []byte(`
FROM busybox
ADD index.html index.html
ENTRYPOINT busybox httpd -f -p 8000
`)
	_ = w.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Mode: 0644,
		Uid:  0,
		Gid:  0,
		Size: int64(len(dockerfile)),
	})

	_, _ = w.Write(dockerfile)

	_ = w.WriteHeader(&tar.Header{
		Name: "index.html",
		Mode: 0644,
		Uid:  0,
		Gid:  0,
		Size: int64(len([]byte(contents))),
	})
	_, _ = w.Write([]byte(contents))
	_ = w.Close()
	return b
}

func decodeFile(path string, ptr interface{}) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	decodeBytes(contents, ptr)
}

func decodeBytes(b []byte, ptr interface{}) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(b), 4096)
	err := decoder.Decode(ptr)
	if err != nil {
		panic(err)
	}
}

func encode(obj interface{}) io.Reader {
	jsonData, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	data, err := yamlEncoder.JSONToYAML(jsonData)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(data)
}

type cmdOption func(*exec.Cmd)

func withStdin(r io.Reader) cmdOption {
	return func(cmd *exec.Cmd) {
		cmd.Stdin = r
	}
}

func cmd(s string, options ...cmdOption) []byte {
	fmt.Println(s)
	cmd := exec.Command("bash", "-c", s)
	for _, o := range options {
		o(cmd)
	}
	stdout, err := cmd.Output()
	if err != nil {
		exitErr, isExitError := err.(*exec.ExitError)
		if isExitError {
			fmt.Println("Stderr: ", string(exitErr.Stderr))
			panic(err)
		}
		panic(err)
	}
	return stdout
}

func config() *rest.Config {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	config, err := loader.ClientConfig()
	if err != nil {
		panic(err.Error())
	}
	return config
}

func client() *kubernetes.Clientset {
	clientset, err := kubernetes.NewForConfig(config())
	if err != nil {
		panic(err.Error())
	}
	return clientset
}

func newInformer(cs *kubernetes.Clientset, gvr schema.GroupVersionResource, options ...informers.SharedInformerOption) cache.SharedInformer {
	factory := informers.NewSharedInformerFactoryWithOptions(cs, 5*time.Second,
		append([]informers.SharedInformerOption{informers.WithNamespace("default")}, options...)...)
	resFactory, err := factory.ForResource(gvr)
	if err != nil {
		panic(err)
	}

	return resFactory.Informer()
}

func runPodInformer(informer cache.SharedInformer, podCallback func(pod *v1.Pod)) {
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				podCallback(pod)
			}
		},
		UpdateFunc: func(_, obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				podCallback(pod)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				podCallback(pod)
			}
		},
	})
	go informer.Run(make(chan struct{}))
}

type portforwardServer struct {
	cs        *kubernetes.Clientset
	pf        *portforward.PortForwarder
	pfPodName string
}

func newPortforwardServer(cs *kubernetes.Clientset) *portforwardServer {
	return &portforwardServer{cs: cs}
}

func (s *portforwardServer) ConnectToPod(pod *v1.Pod, port int) {
	if s.pfPodName == pod.Name {
		return
	}

	if s.pf != nil {
		s.pf.Close()
	}

	color.Green(fmt.Sprintf("Port-forward localhost:%d to %s\n", port, pod.ObjectMeta.Name))
	transport, upgrader, err := spdy.RoundTripperFor(config())
	if err != nil {
		panic(err)
	}

	req := s.cs.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("portforward")

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())
	if err != nil {
		panic(err)
	}

	pf, err := portforward.New(dialer, []string{fmt.Sprintf("%d:8000", port)},
		make(chan struct{}, 1), make(chan struct{}, 1),
		ioutil.Discard, ioutil.Discard)
	if err != nil {
		panic(err)
	}
	s.pf = pf
	s.pfPodName = pod.Name

	go func() {
		_ = pf.ForwardPorts()
	}()
}
