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
retrieve_associated_entity(Retrieve the entity <br> component/application)
determine_snapshot{Does a snapshot exist?}
prep_snapshot(Gather Application components<br> Add new component)
check_chains{Chains annotation present?}
annotate_pipelineRun(Annotate pipeline with <br> name of Snapshot)
add_finalizer(Add finalizer to build PLR)
remove_finalizer(Remove finalizer from build PLR)
error[Return error]
continue[Continue processing]
update_metadata(add PR group info to build pipelineRun metadata)
notify_pr_group_failure(annotate Snapshots and in-flight builds in PR group with failure message)
failed_group_pipeline_run{Pipeline failed?}

%% Node connections
predicate                        --> get_pipeline_run
predicate                       -->  new_pipeline_run
predicate                       -->  new_pipeline_run_without_prgroup
predicate                       -->  failed_pipeline_run
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
get_pipeline_run           --Yes --> retrieve_associated_entity
get_pipeline_run           --No  --> error
retrieve_associated_entity --No  --> error
error                            --> continue
retrieve_associated_entity --Yes --> determine_snapshot
determine_snapshot         --Yes --> annotate_pipelineRun
determine_snapshot         --No  --> prep_snapshot
prep_snapshot                    --> check_chains
check_chains               --Yes --> annotate_pipelineRun
annotate_pipelineRun       --Yes --> remove_finalizer
remove_finalizer                 --> continue

%% Assigning styles to nodes
class predicate Amber;
class error Red;

  ```
