# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.20.10-6 as builder

WORKDIR /opt/app-root/src

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
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

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

ARG ENABLE_WEBHOOKS=true
ENV ENABLE_WEBHOOKS=${ENABLE_WEBHOOKS}
# Use ubi-minimal as minimal base image to package the manager binary
# Refer to https://catalog.redhat.com/software/containers/ubi8/ubi-minimal/5c359a62bed8bd75a2c3fba8 for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.9-1108.1705420507
COPY --from=builder /opt/app-root/src/manager /

# It is mandatory to set these labels
LABEL description="RHTAP Integration Service"
LABEL io.k8s.description="RHTAP Integration Service"
LABEL io.k8s.display-name="Integration-service"
LABEL summary="RHTAP Integration Service"
LABEL io.openshift.tags="rhtap"

USER 65532:65532

ENTRYPOINT ["/manager"]
