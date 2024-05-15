# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.21.9-1.1715774364 as builder

WORKDIR /opt/app-root/src

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY tekton/ tekton/
COPY helpers/ helpers/
COPY gitops/ gitops/
COPY pkg/ pkg/
COPY release/ release/
COPY metrics/ metrics/
COPY status/ status/
COPY git/ git/
COPY loader/ loader/
COPY cache/ cache/
COPY cmd/ cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o snapshotgc cmd/snapshotgc/snapshotgc.go

ARG ENABLE_WEBHOOKS=true
ENV ENABLE_WEBHOOKS=${ENABLE_WEBHOOKS}
# Use ubi-minimal as minimal base image to package the manager binary
# Refer to https://catalog.redhat.com/software/containers/ubi8/ubi-minimal/5c359a62bed8bd75a2c3fba8 for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.9-1161.1715068733
COPY --from=builder /opt/app-root/src/manager /
COPY --from=builder /opt/app-root/src/snapshotgc /

# workaround, fixing glibc CVE which prevents us to release; remove this when new parent image is released
RUN microdnf upgrade -y glibc && microdnf clean all

# It is mandatory to set these labels
LABEL name="integration-service"
LABEL com.redhat.component="konflux-integration-service"
LABEL description="Konflux Integration Service"
LABEL io.k8s.description="Konflux Integration Service"
LABEL io.k8s.display-name="Integration-service"
LABEL summary="Konflux Integration Service"
LABEL io.openshift.tags="konflux"

USER 65532:65532

ENTRYPOINT ["/manager"]
