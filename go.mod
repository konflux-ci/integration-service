module github.com/konflux-ci/integration-service

go 1.25.0

require (
	github.com/konflux-ci/application-api v0.0.0-20260120144946-dc5c619fb3f9
	github.com/konflux-ci/coverport/instrumentation/go v0.0.0-20251127115143-b5207b335f8b
	github.com/konflux-ci/operator-toolkit v0.0.0-20251118152634-b4f41f073069
	github.com/konflux-ci/release-service v0.0.0-20260127184035-c36c56a3c440
	github.com/onsi/ginkgo/v2 v2.27.5
	github.com/onsi/gomega v1.39.0
	github.com/openshift-pipelines/pipelines-as-code v0.34.0
	k8s.io/api v0.35.0
	k8s.io/apimachinery v0.35.0
	k8s.io/client-go v11.0.0+incompatible
	knative.dev/pkg v0.0.0-20250424013628-d5e74d29daa3
	sigs.k8s.io/controller-runtime v0.22.4
)

replace (
	github.com/google/cel-go => github.com/google/cel-go v0.26.1
	k8s.io/api => k8s.io/api v0.35.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.35.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.35.0
	k8s.io/apiserver => k8s.io/apiserver v0.35.0
	// This is to work around errors like package k8s.io/client-go/X provided by k8s.io/client-go at latest version v0.34.0 but not at required version v1.5.2
	// The version of client-go should align with other k8s.io dependencies to avoid compatibility issues.
	k8s.io/client-go => k8s.io/client-go v0.35.0
	k8s.io/component-base => k8s.io/component-base v0.35.0

	knative.dev/pkg => knative.dev/pkg v0.0.0-20240416145024-0f34a8815650
)

require (
	github.com/agiledragon/gomonkey/v2 v2.13.0
	github.com/bradleyfalzon/ghinstallation/v2 v2.17.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v1.4.3
	github.com/google/cel-go v0.26.1
	github.com/google/go-containerregistry v0.20.7
	github.com/google/go-github/v45 v45.2.0
	github.com/prometheus/client_golang v1.23.2
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	github.com/tektoncd/pipeline v1.7.0
	github.com/tonglil/buflogr v1.1.1
	gitlab.com/gitlab-org/api/client-go v0.134.0
	go.uber.org/mock v0.5.2
	go.uber.org/zap v1.27.1
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96
	golang.org/x/oauth2 v0.34.0
	k8s.io/klog/v2 v2.130.1
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4
)

require (
	cel.dev/expr v0.25.1 // indirect
	contrib.go.opencensus.io/exporter/ocagent v0.7.1-0.20230502190836-7399e0f8ee5e // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.2 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.3.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/blendle/zapdriver v1.3.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanphx/json-patch v5.9.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.7.0 // indirect
	github.com/go-git/go-git/v5 v5.16.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/go-github/v75 v75.0.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20260115054156-294ebfa9ad83 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.5 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kevinburke/ssh_config v1.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/operator-framework/operator-lib v0.19.0 // indirect
	github.com/pjbgf/sha1cd v0.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/prometheus/statsd_exporter v0.28.0 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/skeema/knownhosts v1.3.2 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stoewer/go-strcase v1.3.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/mod v0.32.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/api v0.262.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260122232226-8e98ce8d340d // indirect
	google.golang.org/grpc v1.78.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apiextensions-apiserver v0.35.0 // indirect
	k8s.io/apiserver v0.35.0 // indirect
	k8s.io/component-base v0.35.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260127142750-a19766b6e2d4 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.34.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.1 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

replace code.gitea.io/gitea => code.gitea.io/gitea v1.25.2

replace github.com/nektos/act => gitea.com/gitea/act v0.2.66-0.20240924153421-39656e186641
