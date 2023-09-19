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
  report_status_snapshot(Report status of the test into snapshot annotation `test.appstudio.openshift.io/status`)
  report_status(Report status if Snapshot was created <br> for Pull requests)
  check_tests{Check Snapshot <br> passed all tests}
  check_supersede{Does Snapshot need  <br>to be superseded <br> with a composite Snapshot?}
  create_snapshot(Create Snapshot)
  update_status(Update status)
  clean_environment(Clean up ephemeral environment <br> if testing finished)
  error(Return error)
  requeue(Requeue)
  continue(Continue processing)

  %% Node connections
  predicate                                   --> get_resources
  get_resources     --No                      --> error
  get_resources     --Yes                     --> report_status_snapshot
  report_status_snapshot                    ----> report_status
  report_status     --Yes                     --> check_tests
  check_tests       --No                      --> requeue
  check_tests       --Yes                     --> check_supersede
  check_supersede   --yes                     --> create_snapshot
  create_snapshot   --No                      --> requeue
  update_status     --Yes                     --> clean_environment
  check_supersede   --No                      --> update_status
  create_snapshot   --Yes                     --> update_status
  clean_environment --No                      --> requeue
  clean_environment --yes                     ---> continue
  error                                       --> continue

  %% Assigning styles to nodes
  class predicate Amber;
  class error,requeue Red;

  ```