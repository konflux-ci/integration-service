<div align="center"><h1>IntegrationPipeline Controller</h1></div>

```mermaid

%%{init: {'theme':'forest'}}%%
flowchart TD
  %% Defining the styles
  classDef Red fill:#FF9999;
  classDef Amber fill:#FFDEAD;
  classDef Green fill:#BDFFA4;

  %% Node definitions
  predicate((PREDICATE: <br>Integration Pipeline just got<br> Started OR Finished<br> OR marked for Deletion))
  get_resources{Get pipeline, <br> component, <br> & application}
  report_status_snapshot(Report status of the test <br> into snapshot annotation <br> `test.appstudio.openshift.io/status`)
  is_snapshot_of_pr_event{Is <br> Snapshot created<br> for Pull requests?}
  is_plr_finished_or_getting_deleted{Is <br> Integration PLR <br> finished or marked for<br> deletion?}
  remove_finalizer(Remove <br> `test.appstudio.openshift.io/pipelinerun`<br> finalizer)
  error(Return error)
  continue1(Continue processing)

  %% Node connections
  predicate                                   --> get_resources
  get_resources     --No                      --> error
  get_resources     --Yes                     --> report_status_snapshot
  report_status_snapshot                      --> is_snapshot_of_pr_event
  is_snapshot_of_pr_event            --Yes    --> continue1
  is_snapshot_of_pr_event            --No     --> is_plr_finished_or_getting_deleted
  is_plr_finished_or_getting_deleted --Yes    --> remove_finalizer
  is_plr_finished_or_getting_deleted --No     --> continue1
  remove_finalizer                            --> continue1

  %% Assigning styles to nodes
  class predicate Amber;
  class error,requeue Red;

 ```
