# AppStudio Integration Service
The AppStudio Integration Service is a Kubernetes operator to control the integration and testing of AppStudio-managed 
Application Component builds in Red Hat AppStudio.

## Running, building and testing the operator

This operator provides a [Makefile](Makefile) to run all the usual development tasks. This file can be used by cloning
the repository and running `make` over any of the provided targets.

### Running the operator locally

When testing locally (eg. a CRC cluster), the command `make run install` can be used to deploy and run the operator.
If any change has been done in the code, `make manifests generate` should be executed before to generate the new resources
and build the operator.

### Build and push a new image

To build the operator and push a new image to the registry, the following commands can be used:

```shell
$ make docker-build
$ make docker-push
```

These commands will use the default image and tag. To modify them, new values for `TAG` and `IMG` environment variables
can be passed. For example, to override the tag:

```shell
$ TAG=my-tag make docker-build
$ TAG=my-tag make docker-push
```

Or, in the case the image should be pushed to a different repository:

```shell
$ IMG=quay.io/user/integration-service:my-tag make docker-build
$ IMG=quay.io/user/integration-service:my-tag make docker-push
```

### Running tests

To test the code, simply run `make test`. This command will fetch all the required dependencies and test the code. The
test coverage will be reported at the end, once all the tests have been executed.