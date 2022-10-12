# Run integration-service on kcp workspace

## Create a kcp workspace

You can choose to setup kcp locally or request kcp account on `kcp-stable`/`kcp-unstable`

### Request kcp account on kcp-stable/kcp-unstable

Rerfer to [doc](https://docs.google.com/document/d/1a1k807y7xWSgDKgoS2XQSu0AUKV0V5OJPAZSWNYvDXo/edit) to request a account on `kcp-stable`/`kcp-unstable`


### Create a workspce and enter it
```shell
kubectl kcp workspace create test-workspace --enter
```

### Some other workspace command
```shell
# Go to home workspace
kubectl kcp workspace use '~'
# Create a new workspace and enter it immediately
kubectl kcp workspace create test-workspace --enter
# Go to parent workspace
kubectl kcp workspace ..
# Go to a workspace 
kubectl kcp workspace test-workspace
# Get the current workspace name
kubectl kcp workspce current

```

## Prepare a physical cluster with name `mycluster`

## Go to the created kcp workspace `test-workspace`, create a syncer

```shell
kubectl kcp workload sync mycluster --resources configmaps,deployments.apps,secrets,serviceaccounts,releases.appstudio.redhat.com,releaseplans.appstudio.redhat.com,releaseplanadmissions.appstudio.redhat.com,releasestrategies.appstudio.redhat.com,pipelineruns.tekton.dev,applicationsnapshots.appstudio.redhat.com,components.appstudio.redhat.com,applications.appstudio.redhat.com,integrationtestscenarios.appstudio.redhat.com,applicationsnapshotenvironmentbindings.appstudio.redhat.com,environments.appstudio.redhat.com --syncer-image ghcr.io/kcp-dev/kcp/syncer:v0.7.10 --output-file=syncer710.yaml
```

This commands will create a file `syncer710.yaml`.

Add the CRDs you will use to `syncer710.yaml`

Go to physical cluster `mycluster`, apply them

```shell
oc apply -f syncer710.yaml
```

Check the pod `kcp-syncer-mycluster-XXXXX-XXXXXXX` until it is running well

Go to kcp workspace `test-workspace`, get apiresourceschema 

```
kubectl get apiresourceschema -o name -w
```

add them to apiexport.yaml, example [apiexport_integration.yaml](../../kcp/apiexport_integration.yaml)

Apply `apiexport.yaml` to apiexport `integration`

```
kubectl apply -f apiexport.yaml
```

Apply `apibinding_integration.yaml`

```
kubectl apply -f apibinding_integration.yaml
```

Check CRDs

```
kubectl api-resources
```

Start integration service locally

`./bin/manager --api-export-name integration`

Create the resources, here I define the resources in [kcp_test_resource.yaml](kcp_test_resource.yaml)

```shell
oc apply -f kcp_test_resource.yaml
```

Check pipelinerun

```
oc get pipelinerun
```