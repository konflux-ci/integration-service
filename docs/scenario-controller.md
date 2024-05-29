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
  status_check{"IntegrationTestScenario <br>has status<br>IntegrationTestScenarioValid<br>condition undefined<br>or false"}
  update_scenario_status_valid(Update IntegrationTestScenario <br>status to valid)
  update_scenario_status_invalid(Update IntegrationTestScenario <br>status to invalid)
  remove_historical_finalizer(Check for and remove<br>historical IntegrationTestScenario <br> finalizer)
  complete_reconciliation(Complete reconciliation for <br>IntegrationTestScenario)
  continue_reconciliation(Continue with next reconciliation)


  %% Node connections
  predicate                        ---->    |"EnsureCreatedScenarioIsValid()"| remove_historical_finalizer
  remove_historical_finalizer      -->      application_exists
  application_exists               --No-->  update_scenario_status_invalid
  application_exists               --Yes--> set_owner_reference
  set_owner_reference              -->      status_check
  status_check                     --No-->  update_scenario_status_invalid
  status_check                     --Yes--> update_scenario_status_valid
  update_scenario_status_valid     -->      complete_reconciliation
  complete_reconciliation          -->      continue_reconciliation
  update_scenario_status_invalid   -->      continue_reconciliation

   %% Assigning styles to nodes
  class predicate Amber;

  ```