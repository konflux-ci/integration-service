<div align="center"><h1>IntegrationTestScenario Controller</h1></div>

```mermaid

%%{init: {'theme':'forest'}}%%
flowchart TD
  %% Defining the styles
  classDef Red fill:#FF9999;
  classDef Amber fill:#FFDEAD;
  classDef Green fill:#BDFFA4;

  predicate((PREDICATE: <br>Monitor IntegrationTestScenario <br>& filter created events))

  %% Node definitions
  check_sa{"ServiceAccount<br>konflux-integration-runner<br>exists?"}
  create_sa(Create ServiceAccount<br>with ImagePullSecrets)
  check_secret{"Secret<br>components-namespace-pull<br>exists?"}
  create_secret(Create empty<br>dockerconfigjson Secret)
  check_linked{"Secret linked to<br>SA ImagePullSecrets?"}
  link_secret(Update SA to link Secret)
  check_rb{"RoleBinding<br>konflux-integration-runner<br>exists?"}
  create_rb(Create RoleBinding to<br>ClusterRole konflux-integration-runner)
  complete(Complete reconciliation)

  %% Node connections
  predicate                  ---->  |"EnsureIntegrationPipelineServiceAccount()"| check_sa
  check_sa                   --No-->  create_sa
  check_sa                   --Yes--> check_secret
  create_sa                  -->      check_secret
  check_secret               --No-->  create_secret
  check_secret               --Yes--> check_linked
  create_secret              -->      check_linked
  check_linked               --No-->  link_secret
  check_linked               --Yes--> check_rb
  link_secret                -->      check_rb
  check_rb                   --No-->  create_rb
  check_rb                   --Yes--> complete
  create_rb                  -->      complete

  %% Assigning styles to nodes
  class predicate Amber;
  class create_sa,create_secret,link_secret,create_rb Green;

  ```
