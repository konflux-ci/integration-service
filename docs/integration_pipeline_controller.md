<div align="center"><h1>IntegrationPipeline Controller</h1></div>

```mermaid

%%{init: {'theme':'forest'}}%%
flowchart TD
  %% Defining the styles
  classDef Red fill:#FF9999;
  classDef Amber fill:#FFDEAD;
  classDef Green fill:#BDFFA4;

  %% Node definitions
  predicate((PREDICATE: <br>Integration Pipeline <br> reconciliation))
  get_resources{Get pipeline, <br> component, <br> & application}
  report_status_snapshot(Report status of the test <br> into snapshot annotation <br> `test.appstudio.openshift.io/status`)
  report_status(Report status if Snapshot was created <br> for Pull requests)
  clean_environment(Clean up ephemeral environment <br> if testing finished)
  error(Return error)
  requeue(Requeue)
  continue(Continue processing)

  %% Node connections
  predicate                                   --> get_resources
  predicate                                   --> clean_environment
  get_resources     --No                      --> error
  get_resources     --Yes                     --> report_status_snapshot
  report_status_snapshot                    ----> report_status
  clean_environment --No                      --> requeue
  clean_environment --yes                     ---> continue

  %% Assigning styles to nodes
  class predicate Amber;
  class error,requeue Red;

  ```