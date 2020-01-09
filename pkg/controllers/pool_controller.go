// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	poolv1alpha1 "github.com/talos-systems/talos-controller-manager/api/v1alpha1"
	"github.com/talos-systems/talos-controller-manager/pkg/channel"
	"github.com/talos-systems/talos-controller-manager/pkg/constants"
	"github.com/talos-systems/talos-controller-manager/pkg/upgrader"
	"github.com/talos-systems/talos-controller-manager/pkg/version"
)

// PoolReconciler reconciles a Pool object
type PoolReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=upgrade.talos.dev,resources=pools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=upgrade.talos.dev,resources=pools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch

func (r *PoolReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("pool", req.NamespacedName)

	return r.reconcile(ctx, req)
}

func (r *PoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&poolv1alpha1.Pool{}).
		Complete(r)
}

func (r *PoolReconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pool poolv1alpha1.Pool

	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		r.Log.Error(err, "unable to get pool")

		return ctrl.Result{}, err
	}

	if time.Until(pool.Status.NextRun.Time) > pool.Spec.CheckInterval.Duration {
		r.Log.Info("rescheduling next run to checkInterval duration", "checkinterval", pool.Spec.CheckInterval.Duration)

		pool.Status.NextRun = metav1.NewTime(time.Now().UTC().Add(pool.Spec.CheckInterval.Duration))

		if err := r.Update(context.TODO(), &pool); err != nil {
			return r.Result(ctx, req, false), err
		}

		return ctrl.Result{RequeueAfter: pool.Spec.CheckInterval.Duration}, nil
	}

	if pool.Status.NextRun.Time.After(time.Now().UTC()) {
		r.Log.Info("skipping reconciliation, next run is in the future", "pool", pool.Name)
		return ctrl.Result{RequeueAfter: pool.Spec.CheckInterval.Duration}, nil
	}

	v := pool.Spec.Version

	if v == "" {
		// TODO(andrewrynhard): Should these be configurable?
		channels := []channel.Channel{
			channel.LatestChannel,
			channel.EdgeChannel,
			channel.AlphaChannel,
			channel.BetaChannel,
			channel.StableChannel,
		}

		cache := version.NewVersion(&version.V1Alpha1{})

		go func() {
			if err := cache.Run(pool.Spec.Registry, pool.Spec.Repository, channels); err != nil {
				r.Log.Error(err, "version cache failed")
				os.Exit(1)
			}
		}()

		if !cache.WaitForCacheSync() {
			return r.Result(ctx, req, false), fmt.Errorf("timeout waiting for version cache to sync")
		}

		var ok bool

		if v, ok = cache.Get(pool.Spec.Channel); !ok {
			return r.Result(ctx, req, false), fmt.Errorf("no version found for %q channel", pool.Spec.Channel)
		}

		r.Log.Info("obtained version for pool", "version", v, "pool", pool.Name, "channel", pool.Spec.Channel)
	}

	if pool.Status.Version != v {
		pool.Status.Version = v

		if err := r.Update(context.TODO(), &pool); err != nil {
			return r.Result(ctx, req, false), err
		}
	}

	c := &upgrader.Context{
		Client: r.Client,
		Req:    req,
	}

	u := upgrader.NewV1Alpha1(c, pool.Spec.Registry, pool.Spec.Repository)

	policy := upgrader.NewConcurrentPolicy(u, pool.Spec.Concurrency)

	label, err := labels.NewRequirement(constants.V1Alpha1PoolLabel, selection.Equals, []string{pool.Name})
	if err != nil {
		return r.Result(ctx, req, false), err
	}

	opts := &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*label),
	}

	var nodes corev1.NodeList

	if err := r.List(ctx, &nodes, opts); err != nil {
		return r.Result(ctx, req, false), err
	}

	// Update the status.

	pool.Status.Size = len(nodes.Items)

	if err := r.Update(context.TODO(), &pool); err != nil {
		return r.Result(ctx, req, false), err
	}

	// Attempt to continue any existing upgrades.

	poolStatusInProgress := strings.Split(pool.Status.InProgress, ",")

	nodesInProgess := corev1.NodeList{}
	for _, node := range nodes.Items {
		for _, n := range poolStatusInProgress {
			if node.Name == n {
				nodesInProgess.Items = append(nodesInProgess.Items, node)
			}
		}
	}

	if len(nodesInProgess.Items) > 0 {
		r.Log.Info("pool has upgrades in progress", "count", len(nodesInProgess.Items), "pool", pool.Name, "channel", pool.Spec.Channel)
		if err := policy.Run(nodesInProgess, v); err != nil {
			r.Log.Error(err, "upgrade failed")
		}
	}

	// Upgrade all nodes.

	if err := policy.Run(nodes, v); err != nil {
		r.Log.Error(err, "upgrade failed")

		return r.Result(ctx, req, true), err
	}

	return r.Result(ctx, req, false), nil
}

func (r *PoolReconciler) Result(ctx context.Context, req ctrl.Request, fail bool) ctrl.Result {
	var pool poolv1alpha1.Pool

	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}
		}

		r.Log.Error(err, "unable to get pool")

		return ctrl.Result{}
	}

	next := metav1.NewTime(time.Now().UTC().Add(pool.Spec.CheckInterval.Duration))

	defer func() {
		if err := r.Update(ctx, &pool); err != nil {
			r.Log.Error(err, "failed to update pool", "pool", pool.Name)
		}
	}()

	if fail {
		switch pool.Spec.FailurePolicy {
		case "Pause":
			pool.Status.NextRun = metav1.Time{}

			r.Log.Info("pausing upgrades", "pool", pool.Name)

			return ctrl.Result{Requeue: false}
		case "Retry":
			// Nothing to do.
		}
	}

	pool.Status.NextRun = next

	r.Log.Info("requeuing upgrade", "when", next.Time, "pool", pool.Name)

	return ctrl.Result{RequeueAfter: pool.Spec.CheckInterval.Duration}
}
