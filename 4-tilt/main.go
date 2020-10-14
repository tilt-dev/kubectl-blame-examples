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
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/tilt-dev/kubectl-blame-examples/4-tilt/tilt"
	"github.com/tilt-dev/localregistry-go"
	"github.com/tjarratt/babble"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
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

	c := kubernetes.NewForConfigOrDie(config())
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
		fmt.Println(`[go] Adding command = ["sh", "-c", "exit 1"] because --crash=true`)
		deployment.Spec.Template.Spec.Containers[0].Command = []string{"sh", "-c", "exit 1"}
	}

	hash, err := tilt.HashPodTemplateSpec(&deployment.Spec.Template)
	if err != nil {
		panic(err)
	}

	fmt.Printf("[go] Adding template hash so we can trace the pod: %s\n", hash)
	deployment.Spec.Template.ObjectMeta.Labels[tilt.TiltPodTemplateHashLabel] = string(hash)

	out := cmd("kubectl apply -o yaml -f -", withStdin(encode(deployment)))
	deploymentResult := appsv1.Deployment{}
	decodeBytes(out, &deploymentResult)

	uid := deploymentResult.UID
	color.Green(fmt.Sprintf("[go] tilt find pods owned by UID %s\n", uid))

	phases := make(map[string]string)
	containerStatuses := make(map[string]string)
	ignored := make(map[string]bool)

	// Watch for changes
	factory := informers.NewSharedInformerFactoryWithOptions(c, 5*time.Minute,
		informers.WithNamespace("default"))
	resFactory, err := factory.ForResource(v1.SchemeGroupVersion.WithResource("pods"))
	if err != nil {
		panic(err)
	}
	informer := resFactory.Informer()
	ownerFetcher := tilt.NewOwnerFetcher(context.Background(), config())

	done := make(chan bool)
	runPodInformer(informer, func(pod *v1.Pod) {
		tree, err := ownerFetcher.OwnerTreeOf(context.Background(), pod)
		if err != nil {
			log.Printf("error fetching owner tree: %v", err)
			return
		}

		if !tree.ContainsUID(uid) {
			return
		}

		if pod.Labels[tilt.TiltPodTemplateHashLabel] != string(hash) {
			if !ignored[pod.Name] {
				fmt.Printf("Pod: %s | Ignoring | (pod template hash doesn't match)\n", pod.Name)
				ignored[pod.Name] = true
			}
			return
		}

		name := pod.Name
		phase := podPhase(pod)
		cStatus := containerStatus(pod)

		if phases[name] == phase && containerStatuses[name] == cStatus {
			return
		}

		phases[name] = phase
		containerStatuses[name] = cStatus
		fmt.Printf("Pod: %s | Phase: %s | Container: %s | Age: %s\n", name, phase, cStatus, prettyAge(pod))

		if phase == "Running" && cStatus == "Running" {
			color.Green("Success")
			close(done)
		}
	})

	// Wait until deployed
	<-done
}
func podPhase(pod *v1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	return string(pod.Status.Phase)
}

func containerStatus(pod *v1.Pod) string {
	if len(pod.Status.ContainerStatuses) == 0 {
		return ""
	}

	state := pod.Status.ContainerStatuses[0].State
	if state.Waiting != nil {
		return state.Waiting.Reason
	} else if state.Running != nil {
		return "Running"
	} else if state.Terminated != nil {
		return state.Terminated.Reason
	}
	return ""
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
