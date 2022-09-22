#!/usr/bin/env bash

set -e

if ! which kubectl-kcp; then
  echo "kubectl-kcp required on path"
  echo "you can install it with running:"
  echo "    $ git clone https://github.com/kcp-dev/kcp && cd kcp && make install"
  exit 1
fi

ROOT=$(dirname "$(dirname "$(realpath "$0")")")
CRD_DIRECTORY=$(realpath "$ROOT"/config/crd/bases)
KCP_API_EXPORT_FILE=$(realpath "$ROOT"/config/kcp/apiexport_integration.yaml)
KCP_API_EXPORT_HEADER="$(cat << EOF
# This file is generated from CRDs by ./hack/generate-kcp-api.sh script.
# Please do not modify!
apiVersion: apis.kcp.dev/v1alpha1
kind: APIExport
metadata:
  name: integration-api
spec:
  permissionClaims:
    - resource: "pipelineruns"
      group: "tekton.dev"
      identityHash: null
    - resource: "applications"
      group: "appstudio.redhat.com"
      identityHash: null # application-api apiexport
    - resource: "components"
      group: "appstudio.redhat.com"
      identityHash: null # application-api apiexport
    - resource: "applicationsnapshots"
      group: "appstudio.redhat.com"
      identityHash: null # application-api apiexport
    - resource: "environments"
      group: "appstudio.redhat.com"
      identityHash: null # application-api apiexport
    - resource: "applicationsnapshotenvironmentbindings"
      group: "appstudio.redhat.com"
      identityHash: null # application-api apiexport
    - resource: "releases"
      group: "appstudio.redhat.com"
      identityHash: null # release apiexport
    - resource: "releaseplans"
      group: "appstudio.redhat.com"
      identityHash: null # release apiexport
  latestResourceSchemas:
EOF
)"
KCP_API_SCHEMA_FILE=$(realpath "$ROOT"/config/kcp/apiresourceschema_integration.yaml)
REQUIREMENTS="kubectl-kcp md5sum"
SCHEMA_REGEX="md5-[a-f0-9]{32}.*\.appstudio\.redhat\.com"

generate_api_export() {
    echo "$KCP_API_EXPORT_HEADER" > "$KCP_API_EXPORT_FILE"

    grep -Eo "$SCHEMA_REGEX" < "$KCP_API_SCHEMA_FILE" | while IFS= read -r schema; do
        echo "    - ${schema}" >> "$KCP_API_EXPORT_FILE"
    done
}

generate_schemas() {
    rm -rf "$KCP_API_SCHEMA_FILE"

    for crd in $(find "$CRD_DIRECTORY" -name '*.yaml' | sort -V); do
        prefix="md5-$(md5sum "$crd" | awk '{print $1}')"
        kubectl-kcp crd snapshot -f "$crd" --prefix "$prefix" >> "$KCP_API_SCHEMA_FILE"
    done
}

check_requirements() {
    for tool in $REQUIREMENTS; do
        if ! [ -x "$(command -v "$tool")" ]; then
            echo "Error: $tool is not installed" >&2
            if [ "$tool" == "kubectl-kcp" ]; then
                echo "The tool can be installed by running the following command:"
                echo "    $ git clone https://github.com/kcp-dev/kcp && cd kcp && make install"
            fi
            exit 1
        fi
    done
}

check_requirements
generate_schemas
generate_api_export
