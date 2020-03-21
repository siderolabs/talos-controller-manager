// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package upgrader

import (
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ConcurrentPolicy struct {
	Upgrader

	Concurrency int

	log logr.Logger
}

type Job struct {
	req        reconcile.Request
	node       corev1.Node
	version    string
	inProgress bool
}

type Result struct {
	job Job
	err error
}

func NewConcurrentPolicy(u Upgrader, c int) ConcurrentPolicy {
	return ConcurrentPolicy{
		Upgrader:    u,
		Concurrency: c,
		log:         ctrl.Log.WithName("policy").WithName("Concurrent"),
	}
}

func (policy ConcurrentPolicy) Run(req reconcile.Request, nodes corev1.NodeList, version string, inProgress bool) error {
	jobs := make(chan Job, policy.Concurrency)
	results := make(chan Result, len(nodes.Items))

	for w := 0; w < policy.Concurrency; w++ {
		go policy.worker(w, jobs, results)
	}

	for _, node := range nodes.Items {
		jobs <- Job{req, node, version, inProgress}
	}

	close(jobs)

	var result *multierror.Error

	for a := 0; a < len(nodes.Items); a++ {
		r := <-results
		if r.err != nil {
			result = multierror.Append(result, r.err)
		}
	}

	return result.ErrorOrNil()
}

func (policy ConcurrentPolicy) worker(id int, jobs <-chan Job, results chan<- Result) {
	for j := range jobs {
		policy.log.Info("assigned worker to node", "id", id, "node", j.node.Name)

		if err := policy.Upgrade(j.req, j.node, j.version, j.inProgress); err != nil {
			results <- Result{j, err}
		}

		results <- Result{j, nil}
	}
}
