# syntax = docker/dockerfile-upstream:1.1.4-experimental

FROM golang:1.13 AS build
ENV GO111MODULE on
ENV GOPROXY https://proxy.golang.org
ENV CGO_ENABLED 0
WORKDIR /tmp
RUN go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.4
WORKDIR /src
COPY ./go.mod ./
COPY ./go.sum ./
RUN go mod download
RUN go mod verify
COPY ./main.go ./main.go
COPY ./api ./api
COPY ./pkg ./pkg
COPY ./hack ./hack
RUN go list -mod=readonly all >/dev/null
RUN ! go mod tidy -v 2>&1 | grep .

FROM build AS manifests-build
RUN controller-gen rbac:roleName=talos-controller-manager-role crd paths="./..." output:rbac:artifacts:config=hack/config/rbac output:crd:artifacts:config=hack/config/crd
FROM scratch AS manifests
COPY --from=manifests-build /src/hack/config/crd /hack/config/crd
COPY --from=manifests-build /src/hack/config/manager /hack/config/manager
COPY --from=manifests-build /src/hack/config/rbac /hack/config/rbac

FROM build AS generate-build
RUN controller-gen object:headerFile=./hack/boilerplate.go.txt paths="./..."
FROM scratch AS generate
COPY --from=generate-build /src/api /api

FROM k8s.gcr.io/hyperkube:v1.17.0 AS release-build
COPY ./hack ./hack
RUN kubectl kustomize hack/config >/release.yaml
FROM scratch AS release
COPY --from=release-build /release.yaml /release.yaml

FROM build AS binary
RUN --mount=type=cache,target=/root/.cache/go-build GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /talos-controller-manager
RUN chmod +x /talos-controller-manager

FROM scratch AS container
COPY --from=docker.io/autonomy/ca-certificates:febbf49 / /
COPY --from=docker.io/autonomy/fhs:febbf49 / /
COPY --from=binary /talos-controller-manager /talos-controller-manager
ENTRYPOINT [ "/talos-controller-manager" ]
