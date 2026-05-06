# Contributing

- [How to Report Issues](#how-to-report-issues)
- [How to Submit Pull Requests](#how-to-submit-pull-requests)
    - [Development Workflow](#development-workflow)
    - [Pull Request Guidelines](#pull-request-guidelines)
    - [Security basics](#security-basics)
- [Review Process](#review-process)
- [Testing](#testing)


Contributions of all kinds are welcome. In particular pull requests are appreciated. The authors and maintainers will endeavor to help walk you through any issues in the pull request discussion, so please feel free to open a pull request even if you are new to such things.

> [!NOTE]
> This repository is used in [Konflux](https://konflux-ci.dev).
> Therefore, everybody should follow its [Code of Conduct](https://github.com/konflux-ci/community/blob/main/CODE_OF_CONDUCT.md).

## How to Report Issues

- We encourage early communication for all types of contributions.
- Before filing an issue, make sure to check if it is not reported already.
- If the contribution is non-trivial (straightforward bugfixes, typos, etc.), please open an issue to discuss your plans and get guidance from maintainers.

## How to Submit Pull Requests

### Development Workflow

1. **Fork and Clone**: Fork this repository and clone your fork
2. **Create Feature Branch**: Create a new topic branch based on `main`
3. **Make Changes**: Implement your changes. Consult [README.md](README.md) for the structure and development details.
4. **Commit Changes**: See [commit guidelines](#pull-request-guidelines)

### Pull Request Guidelines

**Commit Requirements:**

- Write clear, descriptive commit titles. Should fit under 50 characters
- Write meaningful commit descriptions with each line having less than 72 characters
- Split your contribution into several commits if applicable, each should represent a logical chunk
- Add line `Assisted-by: <name-of-ai-tool>` if you used an AI tool for your contribution
- Sign-off your commits in order to certify that you adhere to [Developer Certificate of Origin](https://developercertificate.org)

**Pull Request Content:**

- **Title**: Clear, descriptive title. Should fit under 72 characters.
- **Description**: Explain the overall changes and their purpose, this should be a cover letter for your commits.
- **Testing**: Describe how the changes were tested
- **Links**: Reference related issues or upstream stories.

**Remember:**

- Konflux is a community project and proper descriptions cannot be replaced by referencing a publicly inaccessible link to Jira or any other private resource.
- Reviewers, other contributors and future generations might not have the same context as you have at the moment of PR submission.

### Security basics

- Never commit secrets or keys to the repository
- Never expose or log sensitive information

## Review Process

**Requirements for Approval:**

- All CI checks pass
- Code review approval from maintainers

**Review Criteria:**

- The contribution follows established patterns and conventions
- Changes are tested and documented
- Security best practices are followed

For any questions or help with contributing, please open an issue or reach out to the maintainers.

## Testing

Tests are written using the *[Ginkgo](https://onsi.github.io/ginkgo/)* framework. Here is some general advice when writing tests:

* When the global `Describe` doesn't add enough context to the tests, use a nested `Describe` or a `Context` to specify what this test alludes to. Contexts should always start with _When_ (i.e. "_When calling the function Foo_")
* Start the descriptions of `It` blocks in lowercase and try to be as descriptive as possible
* Avoid ignoring errors. In other words, make sure all of them are caught and tested
* Files ending with `_suite_test.go` are meant to store the code that is common for tests in the same directory and that will be executed before them. Use these files only if your test setup is big enough to justify it (ie. the suite file has more than the test suite name and the teardown)
* When required, remember to add the required CRDs during the `envtest` setup. See the example [snapshot_suite_test.go](https://github.com/konflux-ci/integration-service/blob/main/internal/controller/snapshot/snapshot_suite_test.go#L70-L91) file for an example of how to configure `CRDDirectoryPaths` in the test environment. This setup is essential for tests to run properly and can save significant debugging time
* After `Create()` or `Update()` objects, use `Get()` before making assurances as the object might be outdated. It is useful after `Delete()` to check if the client returns `errors.IsNotFound`
* Some assurances are likely to require usage of `Eventually` blocks instead of or in addition to `Expect` blocks

### Useful links:

Links that may be used as a starting point:

* [Getting started with Ginkgo](https://onsi.github.io/ginkgo/#getting-started)
* [Gomega reference doc](https://pkg.go.dev/github.com/onsi/gomega#section-readme)
* [Writing controller tests](https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html)
* [Testing your Operator project](https://master.sdk.operatorframework.io/docs/building-operators/golang/testing/)

Unofficial (but useful) links:

* [Ginkgo and Gomega gotchas](https://medium.com/@william.la.martin/ginkgotchas-yeh-also-gomega-13e39185ec96)
* [Effective Ginkgo/Gomega](https://medium.com/swlh/effective-ginkgo-gomega-b6c28d476a09)
