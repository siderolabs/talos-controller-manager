// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package upgrader

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	poolv1alpha1 "github.com/talos-systems/talos-controller-manager/api/v1alpha1"

	"github.com/talos-systems/talos/api/common"
	machineapi "github.com/talos-systems/talos/api/machine"
	"github.com/talos-systems/talos/cmd/osctl/pkg/client"
	talosconstants "github.com/talos-systems/talos/pkg/constants"
	"github.com/talos-systems/talos/pkg/grpc/tls"
	taloskubernetes "github.com/talos-systems/talos/pkg/kubernetes"
	"github.com/talos-systems/talos/pkg/retry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type V1Alpha1 struct {
	log         logr.Logger
	ctrlclient  ctrlclient.Client
	talosclient *client.Client
	kubeclient  *taloskubernetes.Client
}

func NewV1Alpha1(ctrlclient ctrlclient.Client) (v *V1Alpha1, err error) {
	var config *restclient.Config

	config, err = rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	kubeclient, err := taloskubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	var endpoints []string

	endpoints, err = kubeclient.MasterIPs()
	if err != nil {
		return nil, err
	}

	var (
		token string
		ok    bool
	)

	if token, ok = os.LookupEnv("TALOS_TOKEN"); !ok {
		return nil, errors.New("TALOS_TOKEN env var is required")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	certificateProvider, err := tls.NewRemoteRenewingFileCertificateProvider(
		token,
		endpoints,
		talosconstants.TrustdPort,
		[]string{hostname},
		[]net.IP{},
	)
	if err != nil {
		return nil, err
	}

	ca, err := certificateProvider.GetCA()
	if err != nil {
		return nil, fmt.Errorf("failed to get root CA: %w", err)
	}

	creds, err := tls.New(
		tls.WithClientAuthType(tls.Mutual),
		tls.WithCACertPEM(ca),
		tls.WithClientCertificateProvider(certificateProvider),
	)

	talosclient, err := client.NewClient(creds, endpoints, talosconstants.ApidPort)
	if err != nil {
		return nil, fmt.Errorf("error constructing client: %w", err)
	}

	v = &V1Alpha1{
		log:         ctrl.Log.WithName("v1alpha1").WithName("Upgrader"),
		ctrlclient:  ctrlclient,
		talosclient: talosclient,
		kubeclient:  kubeclient,
	}

	return v, nil
}

func (v1alpha1 V1Alpha1) Upgrade(req reconcile.Request, node corev1.Node, tag string, inProgess bool) (err error) {
	var pool poolv1alpha1.Pool
	if err := v1alpha1.ctrlclient.Get(context.Background(), req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	if pool.Spec.Repository == "" {
		return errors.New("a repository is required")
	}

	// TODO(andrewrynhard): This should be passed in.
	image := fmt.Sprintf("docker.io/%s:%s", pool.Spec.Repository, tag)

	// TODO(andrewrynhard): Ensure that we have found the internal address.
	var target string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			target = addr.Address
		}
	}

	// TODO(andrewrynhard): Request upgrade with context timeout.
	ctx := client.WithNodes(context.Background(), target)

	version, err := v1alpha1.getVersion(ctx)
	if err != nil {
		return err
	}

	// TODO(andrewrynhard): Use semantic versioning to figure out if the
	// the node is on an older version.
	upToDate := version.Tag == tag

	switch {
	case upToDate && inProgess:
		// This means that the current operator has become the leader, but
		// another operator initiated the upgrade and failed to remove the
		// in progress status for some reason. So we skip making an upgrade
		// request and try to pick up where the upgrade left off.
		fallthrough
	case !upToDate && inProgess:
		// See above case.
	case upToDate && !inProgess:
		v1alpha1.log.Info("node is up to date", "node", node.Name, "version", version.Tag)
		return nil
	case !upToDate && !inProgess:
		v1alpha1.log.Info("upgrading node", "node", node.Name, "current version", version.Tag, "target version", tag, "installer", image)

		// TODO(andrewrynhard): Remove this.
		time.Sleep(5 * time.Second)

		v1alpha1.log.Info("sending upgrade request", "node", node.Name)

		_, err = v1alpha1.talosclient.Upgrade(ctx, image)
		if err != nil {
			return fmt.Errorf("upgrade request failed: %w", err)
		}

		if err = v1alpha1.setInProgress(req, node.Name); err != nil {
			return err
		}
	}

	logCtx, logCancel := context.WithCancel(ctx)

	defer logCancel()

	// TODO(andrewrynhard): Reconnect and stream logs on error.
	// nolint: errcheck
	go v1alpha1.streamLogs(logCtx, node)

	if err = v1alpha1.verifyUpgrade(ctx, tag, node); err != nil {
		return err
	}

	if err = v1alpha1.cleanup(node); err != nil {
		return err
	}

	if err = v1alpha1.removeInProgress(req, node.Name); err != nil {
		v1alpha1.log.Error(err, "failed to remove node from pool in progress status")
	}

	v1alpha1.log.Info("upgrade successful", "node", node.Name, "version", tag)

	return nil
}

func (v1alpha1 *V1Alpha1) setInProgress(req reconcile.Request, name string) error {
	var pool poolv1alpha1.Pool
	if err := v1alpha1.ctrlclient.Get(context.Background(), req.NamespacedName, &pool); err != nil {
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
	if err := v1alpha1.ctrlclient.Update(context.TODO(), &pool); err != nil {
		return err
	}

	return nil
}

func (v1alpha1 *V1Alpha1) removeInProgress(req reconcile.Request, name string) error {
	var pool poolv1alpha1.Pool
	if err := v1alpha1.ctrlclient.Get(context.Background(), req.NamespacedName, &pool); err != nil {
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
	if err := v1alpha1.ctrlclient.Update(context.TODO(), &pool); err != nil {
		return err
	}

	return nil
}

func (v1alpha1 *V1Alpha1) waitForHealthy(node corev1.Node) (err error) {
	wait := func() error {
		err = retry.Constant(15*time.Minute, retry.WithUnits(3*time.Second), retry.WithJitter(500*time.Millisecond)).Retry(func() error {
			n, err := v1alpha1.kubeclient.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
			if err != nil {
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

func (v1alpha1 *V1Alpha1) getVersion(ctx context.Context) (version *machineapi.VersionInfo, err error) {
	err = retry.Constant(15*time.Minute, retry.WithUnits(3*time.Second), retry.WithJitter(500*time.Millisecond)).Retry(func() error {
		var versions *machineapi.VersionResponse

		versions, err = v1alpha1.talosclient.Version(ctx)
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

func (v1alpha1 *V1Alpha1) streamLogs(ctx context.Context, node corev1.Node) error {
	stream, err := v1alpha1.talosclient.Logs(ctx, "system", common.ContainerDriver_CONTAINERD, "machined", true, 0)
	if err != nil {
		v1alpha1.log.Error(err, "error fetching logs")
	}

	for {
		var data *common.Data
		data, err = stream.Recv()
		if err != nil {
			if err == io.EOF || status.Code(err) == codes.Canceled {
				return nil
			}

			v1alpha1.log.Error(err, "error streaming logs")

			return err
		}

		r := bytes.NewReader(data.Bytes)

		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			v1alpha1.log.Info("upgrade log", "node", node.Name, "log", scanner.Text())
		}
	}
}

func (v1alpha1 *V1Alpha1) cleanup(node corev1.Node) (err error) {
	if err = v1alpha1.kubeclient.Uncordon(node.Name); err != nil {
		v1alpha1.log.Error(err, "failed to undordon node", "node", node.Name)
	}

	v1alpha1.log.Info("node uncordoned", "node", node.Name)

	return nil
}

func (v1alpha1 *V1Alpha1) verifyUpgrade(ctx context.Context, tag string, node corev1.Node) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	for {
		version, err := v1alpha1.getVersion(ctx)
		if err != nil {
			return err
		}

		if version.Tag != tag {
			time.Sleep(10 * time.Second)
			continue
		}

		if err = v1alpha1.waitForHealthy(node); err != nil {
			return fmt.Errorf("node is not healthy: %w", err)
		}

		v1alpha1.log.Info("node is healthy", "node", node.Name)

		return nil
	}
}
