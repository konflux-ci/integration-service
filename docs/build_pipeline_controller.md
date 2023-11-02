<div align="center"><h1>BuildPipeline Controller</h1></div>

```mermaid

%%{init: {'theme':'forest'}}%%
flowchart TD
%% Defining the styles
  classDef Red fill:#FF9999;
  classDef Amber fill:#FFDEAD;
  classDef Green fill:#BDFFA4;

  %% Node definitions
predicate((PREDICATE: <br> Filter events related to <br> PipelineRuns <br> proccessed by Chains <br> that have <br> succeeded))
get_pipeline_run{Pipeline found?}
retrieve_associated_entity(Retrieve the entity <br> component/application)
determine_snapshot{Does a snapshot exist?}
create_snapshot(Gather Application components<br> Add new component  <br> Create snapshot)
annotate_pipelineRun(Annotate pipeline with <br> name of Snapshot)
error[Return error]
continue[Continue processing]

%% Node connections
predicate                        --> get_pipeline_run
get_pipeline_run           --Yes --> retrieve_associated_entity
get_pipeline_run           --No  --> error
retrieve_associated_entity --No  --> error
error                            --> continue
retrieve_associated_entity --Yes --> determine_snapshot
determine_snapshot         --Yes --> annotate_pipelineRun
determine_snapshot         --No  --> create_snapshot
create_snapshot            --Yes --> annotate_pipelineRun
annotate_pipelineRun       --Yes --> continue

%% Assigning styles to nodes
class predicate Amber;
class error Red;

  ```