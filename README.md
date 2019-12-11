# talos-controller-manager

## Getting Started

```bash
kubectl label node -l node-role.kubernetes.io/master='' v1alpha1.upgrade.talos.dev/pool=serial-latest
kubectl label node -l node-role.kubernetes.io/worker='' v1alpha1.upgrade.talos.dev/pool=concurrent-latest
```

```bash
export TOKEN=<token>
cat <<EOF >./hack/config/examples/env.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: talos-controller-manager
spec:
  template:
    spec:
      containers:
        - name: talos-controller-manager
          env:
            - name: TALOS_TOKEN
              value: $TOKEN
EOF
```

```bash
kubectl apply -k ./hack/config/examples
```

```bash
kubectl get pods -n talos-system
```

```bash
kubectl logs -n talos-system -f $(kubectl get lease -n talos-system talos-controller-manager -o jsonpath='{.spec.holderIdentity}')
```
