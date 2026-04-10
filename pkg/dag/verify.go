/*
Copyright 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dag

import (
	"fmt"
	"maps"

	"github.com/konflux-ci/integration-service/api/v1beta2"
)

// Node represents a Task in a pipeline.
type Node struct {
	// Key represent a unique name of the node in a graph
	Key string
	// Prev represent all the Previous task Nodes for the current Task
	Prev []*Node
	// Next represent all the Next task Nodes for the current Task
	Next []*Node
	// FailFastMap maps next node names to their FailFast behavior
	FailFastMap map[string]bool
}

// Graph represents the Pipeline Graph
type Graph struct {
	// Nodes represent map of PipelineTask name to Node in Pipeline Graph
	Nodes map[string]*Node
}

func newGraph() *Graph {
	return &Graph{Nodes: map[string]*Node{}}
}

func (g *Graph) addScenarioToGraph(scenarioName string) {
	newNode := &Node{
		Key: scenarioName,
	}
	g.Nodes[scenarioName] = newNode
}

// ValidateTestGraph checks that all scenarios are representable as a DAG within
// the provided testGraph (i.e. the graph contains no cycles).
// TODO: when we validate test graph during testing time, we'll wan to expand the
// for loop in this function to also remove nodes from graphCopy that are not going
// to be run based on the contexts
func ValidateTestGraph(testGraph map[string][]v1beta2.TestGraphNode, scenarios ...v1beta2.IntegrationTestScenario) error {
	dag := newGraph()
	graphCopy := maps.Clone(testGraph)

	for _, scenario := range scenarios {
		dag.addScenarioToGraph(scenario.Name)

		// Ensure every scenario appears in testGraph even if it has no declared
		// parents, so that checkGraphForCycles sees the full node set.
		// This isn't technically necessary (every node in a cycle has a parent)
		// but it creates some peace of mind
		if _, ok := graphCopy[scenario.Name]; !ok {
			graphCopy[scenario.Name] = []v1beta2.TestGraphNode{}
		}
	}

	// We're technically checking a reversed version of the graph. Reversing
	// the graph preserves all cycles so this is acceptable
	if err := checkGraphForCycles(graphCopy); err != nil {
		return fmt.Errorf("invalid TestGraph - cycle detected: %w", err)
	}
	return nil
}

// checkGraphForCycles uses Kahn's algorithm on the reversed dependency graph to
// detect cycles.  The testGraph maps each scenario name to the list of parent
// scenarios it depends on (parents must finish before the keyed scenario starts).
func checkGraphForCycles(testGraph map[string][]v1beta2.TestGraphNode) error {
	// indegrees[x] = number of nodes that list x as a parent (i.e. x's child count).
	indegrees := make(map[string]int)
	var res []string

	for _, parentNodes := range testGraph {
		for _, parentNode := range parentNodes {
			indegrees[parentNode.Name]++
		}
	}

	// Seed the queue with nodes that have no children (nobody depends on them).
	var queue []string
	for node := range testGraph {
		if indegrees[node] == 0 {
			queue = append(queue, node)
			res = append(res, node)
		}
	}

	// BFS: for each childless node, decrement its parents' child-counts; enqueue
	// any parent that reaches zero.
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, parentNode := range testGraph[node] {
			indegrees[parentNode.Name]--
			if indegrees[parentNode.Name] == 0 {
				queue = append(queue, parentNode.Name)
				res = append(res, parentNode.Name)
			}
		}
	}

	switch {
	case len(res) < len(testGraph):
		return fmt.Errorf("cycles found in graph; remaining indegrees: %+v", indegrees)
	default:
		return nil
	}
}
