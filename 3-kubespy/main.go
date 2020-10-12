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
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/tilt-dev/kubecon-2020-talk/3-kubespy/kubespy"
	"github.com/tilt-dev/localregistry-go"
	"github.com/tjarratt/babble"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

	_ = cmd("kubectl apply -o yaml -f -", withStdin(encode(deployment)))

	color.Green(fmt.Sprintf("[go] kubespy trace %s\n", deployment.Name))
	kubespy.TraceDeployment("default", deployment.Name)
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
