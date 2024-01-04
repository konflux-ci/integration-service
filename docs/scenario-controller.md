<div align="center"><h1>IntegrationTestScenario Controller</h1></div>

```mermaid

%%{init: {'theme':'forest'}}%%
flowchart TD
  %% Defining the styles
  classDef Red fill:#FF9999;
  classDef Amber fill:#FFDEAD;
  classDef Green fill:#BDFFA4;

  predicate((PREDICATE: <br>Monitor IntegratonTestScenario <br>& filter created/updated/deleted <br>events for the resource ))
  %%%%%%%%%%%%%%%%%%%%%%% Drawing EnsureCreatedScenarioIsValid() function

  %% Node definitions
  
  application_exists{"Application for scenario <br>was found?"}
  set_owner_reference(Set owner reference to <br>IntegrationTestScenario <br> if not already existing)
  environment_defined{"IntegrationTestScenario <br>has environment defined?"}
  environment_exists{"Environment exists <br> in same namespace <br> as IntegrationTestScenario?"}
  update_scenario_status_valid(Update IntegrationTestScenario <br>status to valid)
  update_scenario_status_invalid(Update IntegrationTestScenario <br>status to invalid)
  get_any_environments_for_scenario{Get any existing ephemeral Environments for the <br>IntegrationTestScenario}
  cleanup_all_found_environments(Clean up all found ephemeral Environments)
  remove_finalizer(Remove the finalizer from IntegrationTestScenario)
  complete_reconciliation(Complete reconciliation for <br>IntegrationTestScenario)
  continue_reconciliation(Continue with next reconciliation)


  %% Node connections
  predicate                        ---->    |"EnsureCreatedScenarioIsValid()"| application_exists
  application_exists               --No-->  update_scenario_status_invalid
  application_exists               --Yes--> set_owner_reference
  set_owner_reference              -->      environment_defined
  environment_defined              --Yes--> environment_exists
  environment_defined              --No-->  update_scenario_status_valid
  environment_exists               --No-->  update_scenario_status_invalid
  environment_exists               --Yes--> update_scenario_status_valid
  update_scenario_status_valid     -->      complete_reconciliation
  complete_reconciliation          -->      continue_reconciliation
  update_scenario_status_invalid   -->      continue_reconciliation

%% Node connections
predicate                          -->      |"EnsureDeletedScenarioResourcesAreCleanedUp()"| get_any_environments_for_scenario
get_any_environments_for_scenario  --Yes--> cleanup_all_found_environments
get_any_environments_for_scenario  --No-->  remove_finalizer
cleanup_all_found_environments     -->      remove_finalizer
remove_finalizer                   -->      continue_reconciliation

   %% Assigning styles to nodes
  class predicate Amber;

  ```