# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:9.8-1782377916@sha256:17c888d75753f128f6cbdc5587932c3abd2632ca8e0931aa27b9a60c7a75ac62 as builder

USER 1001

WORKDIR /opt/app-root/src

# Copy the Go Modules manifests
COPY --chown=1001:0 go.mod go.mod
COPY --chown=1001:0 go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
#RUN go mod download

# Copy the go source
COPY --chown=1001:0 . .

# Build arguments
ARG ENABLE_COVERAGE=false

# Build with or without coverage instrumentation
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "Building with coverage instrumentation..."; \
        CGO_ENABLED=0 go build -cover -covermode=atomic -tags=coverage -o manager ./cmd; \
    else \
        echo "Building production binary..."; \
        CGO_ENABLED=0 go build -a -o manager ./cmd; \
    fi \
 && CGO_ENABLED=0 go build -a -o snapshotgc cmd/snapshotgc/snapshotgc.go

ARG ENABLE_WEBHOOKS=true
ENV ENABLE_WEBHOOKS=${ENABLE_WEBHOOKS}
# Use ubi-minimal as minimal base image to package the manager binary
# Refer to https://catalog.redhat.com/software/containers/ubi9/ubi-minimal/615bd9b4075b022acc111bf5 for more details
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8-1782366411
COPY --from=builder /opt/app-root/src/manager /
COPY --from=builder /opt/app-root/src/snapshotgc /

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
