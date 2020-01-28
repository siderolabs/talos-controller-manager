// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	poolv1alpha1 "github.com/talos-systems/talos-controller-manager/api/v1alpha1"
	"github.com/talos-systems/talos-controller-manager/pkg/controllers"
	"github.com/talos-systems/talos-controller-manager/pkg/upgrader"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = poolv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func run(mgr manager.Manager, leaseName, namespace string) (err error) {
	var config *rest.Config

	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	if ok {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return err
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	id, err := os.Hostname()
	if err != nil {
		return err
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name: leaseName,
			// TODO(andrewrynhard): Get the namespace from an env var.
			Namespace: namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderelection.RunOrDie(context.Background(), leaderelection.LeaderElectionConfig{
		Lock: lock,
		// IMPORTANT: you MUST ensure that any code you have that
		// is protected by the lease must terminate **before**
		// you call cancel. Otherwise, you could have a background
		// loop still running and another process could
		// get elected before your background loop finished, violating
		// the stated goal of the lease.
		ReleaseOnCancel: true,
		LeaseDuration:   30 * time.Second,
		RenewDeadline:   15 * time.Second,
		RetryPeriod:     5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				setupLog.Info("starting manager")
				if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
					setupLog.Error(err, "problem running manager")
					os.Exit(1)
				}
			},
			OnStoppedLeading: func() {
				log.Println("lost leadership")
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					return
				}
				log.Printf("new leader elected: %s", identity)
			},
		},
	})

	return nil
}

func main() {
	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		// We disable the builtin leader election in favor of using the leases API.
		LeaderElection: false,
		Port:           9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	u, err := upgrader.NewV1Alpha1(mgr.GetClient())
	if err != nil {
		setupLog.Error(err, "unable to create upgrader")
		os.Exit(1)
	}

	if err = (&controllers.PoolReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("Pool"),
		Upgrader: u,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pool")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := run(mgr, "talos-controller-manager", "talos-system"); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
