<div align="center"><h1>StatusReport Controller</h1></div>

```mermaid
%%{init: {'theme':'forest'}}%%
flowchart TD
  %% Defining the styles
    classDef Amber fill:#FFDEAD;

  predicate((PREDICATE: <br>Snapshot has annotation <br>test.appstudio.openshift.io/status <br>changed))

  %%%%%%%%%%%%%%%%%%%%%%% Drawing EnsureSnapshotTestStatusReported() function 

  %% Node definitions
  ensure(Process further if: Snapshot has label <br>pac.test.appstudio.openshift.io/git-provider:github <br>defined)
  get_annotation_value(Get integration test status from annotation <br>test.appstudio.openshift.io/status <br>from Snapshot)
  collect_commit_info(Collect commit owner, repo and SHA from Snapshot)

  is_installation_defined{Is annotation <br>pac.test.appstudio.openshift.io/installation-id <br>defined?}

  create_appInstallation_token(Create github application installation token)
  get_all_checkRuns_from_gh(Get all checkruns from github <br>according to <br>commit owner, repo and SHA)
  create_checkRunAdapter(Create checkRun adapter according to <br>commit owner, repo, SHA <br>and integration test status)
  does_checkRun_exist{Does checkRun exist <br>on github already?}
  create_new_checkRun_on_gh(Create new checkrun on github)
  is_checkRun_update_needed{Does existing checkRun <br>have different text?}
  update_existing_checkRun_on_gh(Update existing checkRun on github)

  set_oAuth_token(Get token from Snapshot and set oAuth token)
  get_all_commitStatuses_from_gh(Get all commitStatuses from github <br>according to commit owner, repo and SHA)
  create_commitStatusAdapter(Create commitStatusAdapter according to <br>commit owner, repo, SHA <br>and integration test status)
  does_commitStatus_exist{Does commitStatus exist <br>on github already?}
  create_new_commitStatus_on_gh(Create new commitStatus on github)

  continue_processing(Controller continues processing)

  %% Node connections
  predicate                      ---->    |"EnsureSnapshotTestStatusReported()"|ensure
  ensure                         -->      get_annotation_value
  get_annotation_value           -->      collect_commit_info
  collect_commit_info            --> is_installation_defined
  is_installation_defined        --Yes--> create_appInstallation_token
  is_installation_defined        --No--> set_oAuth_token

  create_appInstallation_token   --> get_all_checkRuns_from_gh
  get_all_checkRuns_from_gh      --> create_checkRunAdapter
  create_checkRunAdapter         --> does_checkRun_exist
  does_checkRun_exist            --Yes--> is_checkRun_update_needed
  does_checkRun_exist            --No--> create_new_checkRun_on_gh
  create_new_checkRun_on_gh      --> continue_processing
  is_checkRun_update_needed      --Yes--> update_existing_checkRun_on_gh
  is_checkRun_update_needed      --No--> continue_processing
  update_existing_checkRun_on_gh --> continue_processing

  set_oAuth_token                --> get_all_commitStatuses_from_gh
  get_all_commitStatuses_from_gh --> create_commitStatusAdapter
  create_commitStatusAdapter     --> does_commitStatus_exist
  does_commitStatus_exist        --Yes--> continue_processing
  does_commitStatus_exist        --No--> create_new_commitStatus_on_gh
  create_new_commitStatus_on_gh  --> continue_processing

  %% Assigning styles to nodes
  class predicate Amber;
```