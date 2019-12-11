// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package upgrader

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	poolv1alpha1 "github.com/talos-systems/talos-controller-manager/api/v1alpha1"
	"github.com/talos-systems/talos-controller-manager/pkg/constants"

	"github.com/talos-systems/talos/api/common"
	machineapi "github.com/talos-systems/talos/api/machine"
	"github.com/talos-systems/talos/cmd/osctl/pkg/client"
	talosconstants "github.com/talos-systems/talos/pkg/constants"
	"github.com/talos-systems/talos/pkg/crypto/x509"
	"github.com/talos-systems/talos/pkg/grpc/gen"
	taloskubernetes "github.com/talos-systems/talos/pkg/kubernetes"
	"github.com/talos-systems/talos/pkg/retry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type V1Alpha1 struct {
	registry   string
	repository string
	c          *Context
}

type Context struct {
	Client ctrlclient.Client
	Req    ctrl.Request
}

func NewV1Alpha1(c *Context, registry, repository string) *V1Alpha1 {
	return &V1Alpha1{
		registry:   registry,
		repository: repository,
		c:          c,
	}
}

func (v1alpha1 V1Alpha1) Upgrade(node corev1.Node, tag string) (err error) {
	var config *restclient.Config

	config, err = rest.InClusterConfig()
	if err != nil {
		return err
	}

	h, err := taloskubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	var endpoints []string

	endpoints, err = h.MasterIPs()
	if err != nil {
		return err
	}

	if v1alpha1.repository == "" {
		return errors.New("a repository is required")
	}

	// TODO(andrewrynhard): This should be passed in.
	image := fmt.Sprintf("docker.io/%s:%s", v1alpha1.repository, tag)

	var generator *gen.RemoteGenerator

	var (
		token string
		ok    bool
	)

	if token, ok = os.LookupEnv("TALOS_TOKEN"); !ok {
		return errors.New("TALOS_TOKEN env var is required")
	}

	generator, err = gen.NewRemoteGenerator(token, endpoints, talosconstants.TrustdPort)
	if err != nil {
		return errors.Wrap(err, "failed to create trustd client")
	}

	var (
		csr      *x509.CertificateSigningRequest
		identity *x509.PEMEncodedCertificateAndKey
	)

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	csr, identity, err = x509.NewCSRAndIdentity([]string{hostname}, []net.IP{})
	if err != nil {
		return err
	}

	ca, crt, err := generator.Identity(csr)
	if err != nil {
		return err
	}

	identity.Crt = crt

	creds := client.NewClientCredentials(ca, identity.Crt, identity.Key)

	// TODO(andrewrynhard): Ensure that we have found the internal address.
	var target string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			target = addr.Address
		}
	}

	c, err := client.NewClient(creds, []string{target}, talosconstants.ApidPort)
	if err != nil {
		return fmt.Errorf("error constructing client: %w", err)
	}
	// nolint: errcheck
	defer c.Close()

	// TODO(andrewrynhard): Request upgrade with context timeout.
	ctx := context.TODO()

	version, err := getVersion(ctx, c)
	if err != nil {
		return err
	}

	_, inProgess := node.ObjectMeta.Annotations[constants.InProgressAnnotation]
	// TODO(andrewrynhard): Use semantic versioning to figure out if the
	// the node is on an older version.
	upToDate := version.Tag == tag

	switch {
	case upToDate && inProgess:
		// This means that the current operator has become the leader, but
		// another operator initiated the upgrade and failed to remove the
		// annotation for some reason. So we skip making an upgrade request
		// and try to pick up where the upgrade left off.
		fallthrough
	case !upToDate && inProgess:
		// See above case.
	case upToDate && !inProgess:
		log.Printf("node %q is up to date: %s", node.Name, version.Tag)
		return nil
	case !upToDate && !inProgess:
		log.Printf("upgrading node %q from %q to %q via %q", node.Name, version.Tag, tag, image)

		log.Printf("adding annotation to %q", node.Name)
		if err = annotate(node, config, false); err != nil {
			return err
		}

		// TODO(andrewrynhard): Remove this.
		time.Sleep(5 * time.Second)

		log.Printf("sending upgrade request to %q", node.Name)
		_, err = c.Upgrade(ctx, image)
		if err != nil {
			return fmt.Errorf("upgrade request failed: %w", err)
		}

		if err = v1alpha1.setInProgress(node.Name); err != nil {
			return err
		}
	}

	logCtx, logCancel := context.WithCancel(ctx)

	defer logCancel()

	// TODO(andrewrynhard): Reconnect and stream logs on error.
	// nolint: errcheck
	go streamLogs(logCtx, c, node)

	if err = verifyUpgrade(c, config, tag, node); err != nil {
		return err
	}

	if err = cleanup(h, config, node); err != nil {
		return err
	}

	if err = v1alpha1.removeInProgress(node.Name); err != nil {
		log.Println("WARNING: failed to remove node from pool in progress status:", err)
	}

	log.Printf("upgrade of node %q to %q finished successfully", node.Name, tag)

	return nil
}

func (v1alpha1 *V1Alpha1) setInProgress(name string) error {
	var pool poolv1alpha1.Pool
	if err := v1alpha1.c.Client.Get(context.Background(), v1alpha1.c.Req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	f := func(c rune) bool {
		return c == ','
	}

	nodes := strings.FieldsFunc(pool.Status.InProgress, f)
	nodes = append(nodes, name)
	pool.Status.InProgress = strings.Join(nodes, ",")
	if err := v1alpha1.c.Client.Update(context.TODO(), &pool); err != nil {
		return err
	}

	return nil
}

func (v1alpha1 *V1Alpha1) removeInProgress(name string) error {
	var pool poolv1alpha1.Pool
	if err := v1alpha1.c.Client.Get(context.Background(), v1alpha1.c.Req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	nodes := strings.Split(pool.Status.InProgress, ",")
	tmp := []string{}
	for _, node := range nodes {
		if node == name {
			continue
		}

		tmp = append(tmp, node)
	}
	pool.Status.InProgress = strings.Join(tmp, ",")
	if err := v1alpha1.c.Client.Update(context.TODO(), &pool); err != nil {
		return err
	}

	return nil
}

func annotate(node corev1.Node, config *restclient.Config, remove bool) (err error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	oldData, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal unmodified node %q into JSON: %w", node.Name, err)
	}

	if remove {
		delete(node.ObjectMeta.Annotations, constants.InProgressAnnotation)
	} else {
		// TODO(andrewrynhard): We should drop a JSON blob here that holds which
		// version the node is upgrading from and to.
		node.ObjectMeta.Annotations[constants.InProgressAnnotation] = ""
	}

	newData, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal modified node %q into JSON: %w", node.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, corev1.Node{})
	if err != nil {
		return fmt.Errorf("failed to create two way merge patch: %w", err)
	}

	_, err = clientset.CoreV1().Nodes().Patch(node.Name, types.StrategicMergePatchType, patchBytes)

	return err
}

func waitForHealthy(node corev1.Node, config *restclient.Config) (err error) {
	wait := func() error {
		err = retry.Constant(15*time.Minute, retry.WithUnits(3*time.Second), retry.WithJitter(500*time.Millisecond)).Retry(func() error {
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				log.Println(err)
				return retry.ExpectedError(err)
			}

			n, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			if err != nil {
				log.Println(err)
				return retry.ExpectedError(err)
			}

			for _, condition := range n.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					if condition.Status == corev1.ConditionFalse {
						return retry.ExpectedError(errors.New("node not ready"))
					}
				}
			}

			return nil
		})

		return err
	}

	for i := 0; i < 3; i++ {
		if err = wait(); err != nil {
			return err
		}

		time.Sleep(10 * time.Second)
	}

	return nil
}

func getVersion(ctx context.Context, client *client.Client) (version *machineapi.VersionInfo, err error) {
	err = retry.Constant(15*time.Minute, retry.WithUnits(3*time.Second), retry.WithJitter(500*time.Millisecond)).Retry(func() error {
		var versions *machineapi.VersionResponse

		versions, err = client.Version(ctx)
		if err != nil {
			return retry.ExpectedError(err)
		}

		version = versions.Messages[0].Version

		return nil
	})

	if err != nil {
		return nil, err
	}

	return version, nil
}

func streamLogs(ctx context.Context, client *client.Client, node corev1.Node) error {
	stream, err := client.Logs(ctx, "system", common.ContainerDriver_CONTAINERD, "machined", true)
	if err != nil {
		log.Printf("error fetching logs: %s", err)
	}

	for {
		var data *common.Data
		data, err = stream.Recv()
		if err != nil {
			if err == io.EOF || status.Code(err) == codes.Canceled {
				return nil
			}

			log.Printf("error streaming logs: %s", err)

			return err
		}

		r := bytes.NewReader(data.Bytes)

		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			log.Printf("%s: %s", node.Name, scanner.Text())
		}
	}
}

func cleanup(h *taloskubernetes.Client, config *restclient.Config, node corev1.Node) (err error) {
	if err = h.Uncordon(node.Name); err != nil {
		log.Printf("warning: failed to undordon %q: %v", node.Name, err)
	}

	log.Printf("uncordoned node %q", node.Name)

	if err = annotate(node, config, true); err != nil {
		return err
	}

	log.Printf("removed annotation from %q", node.Name)

	return nil
}

func verifyUpgrade(client *client.Client, config *restclient.Config, tag string, node corev1.Node) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for {
		version, err := getVersion(ctx, client)
		if err != nil {
			return err
		}

		if version.Tag != tag {
			time.Sleep(10 * time.Second)
			continue
		}

		if err = waitForHealthy(node, config); err != nil {
			return fmt.Errorf("node is not healthy: %w", err)
		}

		log.Printf("node %q is healthy", node.Name)

		return nil
	}
}
