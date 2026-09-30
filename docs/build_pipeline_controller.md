<div align="center"><h1>BuildPipeline Controller</h1></div>

```mermaid

%%{init: {'theme':'forest'}}%%
flowchart TD
%% Defining the styles
  classDef Red fill:#FF9999;
  classDef Amber fill:#FFDEAD;
  classDef Green fill:#BDFFA4;

  %% Node definitions
predicate((PREDICATE: <br> Filter events related to <br> PipelineRuns))
new_pipeline_run{Pipeline created?}
new_pipeline_run_without_prgroup{PR group is added to pipelineRun metadata?}
get_pipeline_run{Pipeline updated?}
failed_pipeline_run{Pipeline failed?}
finalizer_exists{Does the finalizer already exist?}
need_to_set_integration_test{Build pipelineRun is newly triggered?<br>Or Build pipelineRun failed?<br> Or Failing to create snapshot?}
select_build_model{Which build model is in use?}
retrieve_legacy_entities(Retrieve component and application<br>for a legacy application build)
retrieve_component_groups(Retrieve ComponentGroups<br>for a component-group build)
component_group_operation{Does this ComponentGroup operation<br>require the component?}
retrieve_component_for_operation(Retrieve component<br>when required by the operation)
determine_snapshot{Does a snapshot exist?}
prep_snapshot(Gather Application or ComponentGroup components<br> Add new component)
check_chains{Chains annotation present?}
annotate_pipelineRun(Annotate pipeline with <br> name of Snapshot)
add_finalizer(Add finalizer to build PLR)
remove_finalizer(Remove finalizer from build PLR)
error[Return error]
continue[Continue processing]
update_metadata(add PR group info to build pipelineRun metadata)
notify_pr_group_failure(annotate Snapshots and in-flight builds in PR group with failure message)
failed_group_pipeline_run{Pipeline failed?}
report_component_integration_test_status(Report component snapshot<br>integration test status<br>to git provider)
group_snapshot_enforcement_annotation{Application or ComponentGroup has<br>integration.konflux-ci.dev/always-create-group-snapshots: true?}
check_group_snapshot_eligibility{Is group snapshot expected<br>for this PR group?<br>(>= 2 components with open PR/MR)}
report_group_integration_test_status(Report group snapshot<br>integration test status<br>to git provider)
update_build_plr_annotation(Update build pipelineRun annotation<br>test.appstudio.openshift.io/snapshot-creation-report<br>with the status)
successful_pipeline(Successful build pipelinerun<br>from push event and is signed)
update_GCL(Update Global Candidate List for the built component)

%% Node connections
predicate                        --> get_pipeline_run
predicate                       -->  new_pipeline_run
predicate                       -->  new_pipeline_run_without_prgroup
predicate                       -->  failed_pipeline_run
predicate                       -->  need_to_set_integration_test
predicate                       -->  successful_pipeline
new_pipeline_run           --Yes-->  finalizer_exists
finalizer_exists           --No-->   add_finalizer
add_finalizer                    --> continue
failed_pipeline_run        --Yes --> remove_finalizer
new_pipeline_run_without_prgroup --No  --> update_metadata
new_pipeline_run_without_prgroup --Yes  --> failed_group_pipeline_run
failed_group_pipeline_run  --Yes --> notify_pr_group_failure
failed_group_pipeline_run   --No --> continue
notify_pr_group_failure          --> continue
update_metadata                  --> continue
get_pipeline_run           --Yes --> select_build_model
get_pipeline_run           --No  --> error
select_build_model  --Legacy application--> retrieve_legacy_entities
select_build_model  --ComponentGroup--> retrieve_component_groups
retrieve_legacy_entities --No  --> error
retrieve_component_groups --No  --> error
error                            --> continue
retrieve_legacy_entities --Yes --> determine_snapshot
retrieve_component_groups --Yes --> determine_snapshot
determine_snapshot         --Yes --> annotate_pipelineRun
determine_snapshot         --No  --> prep_snapshot
prep_snapshot                    --> check_chains
check_chains               --Yes --> annotate_pipelineRun
annotate_pipelineRun       --Yes --> remove_finalizer
remove_finalizer                 --> continue
need_to_set_integration_test  --Yes --> component_group_operation
need_to_set_integration_test  --No  --> continue
component_group_operation --Yes --> retrieve_component_for_operation
component_group_operation --No --> report_component_integration_test_status
retrieve_component_for_operation --> report_component_integration_test_status
report_component_integration_test_status --> group_snapshot_enforcement_annotation
group_snapshot_enforcement_annotation --Yes--> report_group_integration_test_status
group_snapshot_enforcement_annotation --No --> check_group_snapshot_eligibility
check_group_snapshot_eligibility --Yes--> report_group_integration_test_status
check_group_snapshot_eligibility --No --> update_build_plr_annotation
report_group_integration_test_status --> update_build_plr_annotation
update_build_plr_annotation --> continue

successful_pipeline --> update_GCL
update_GCL          --> continue

%% Assigning styles to nodes
class predicate Amber;
class error Red;

  ```
