FROM scratch
COPY --from=docker.io/autonomy/ca-certificates:febbf49 / /
COPY --from=docker.io/autonomy/fhs:febbf49 / /
COPY ./talos-controller-manager /talos-controller-manager
ENTRYPOINT [ "/talos-controller-manager" ]
