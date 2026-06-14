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
	"slices"
	"sort"
	"strings"

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

	adj := make(map[string][]string, len(graphCopy))
	for node, parents := range graphCopy {
		for _, p := range parents {
			adj[node] = append(adj[node], p.Name)
		}
		if _, ok := adj[node]; !ok {
			adj[node] = nil
		}
	}

	if err := checkGraphForCycles(adj); err != nil {
		return fmt.Errorf("invalid TestGraph - cycle detected: %w", err)
	}
	return nil
}

// ValidateNudgeGraph checks that the nudge relationships form a DAG (no cycles).
func ValidateNudgeGraph(nudges []v1beta2.NudgeRelationship) error {
	if len(nudges) == 0 {
		return nil
	}

	adj := make(map[string][]string)
	for _, n := range nudges {
		adj[n.From] = append(adj[n.From], n.To)
		if _, ok := adj[n.To]; !ok {
			adj[n.To] = nil
		}
	}

	if err := checkGraphForCycles(adj); err != nil {
		return fmt.Errorf("invalid NudgeGraph - cycle detected: %w", err)
	}
	return nil
}

// checkGraphForCycles uses depth-first search (DFS) to detect cycles in a directed graph represented
// as an adjacency list. Returns an error containing the cycle path if found.
func checkGraphForCycles(adj map[string][]string) error {
	if cycle, found := findCycleInAdjacencyList(adj); found {
		return fmt.Errorf("%s", formatCyclePath(cycle))
	}
	return nil
}

const (
	colorWhite = 0 // unvisited
	colorGray  = 1 // in current DFS path
	colorBlack = 2 // fully processed
)

// findCycleInAdjacencyList performs DFS to detect cycles and returns the cycle path.
func findCycleInAdjacencyList(adj map[string][]string) ([]string, bool) {
	color := make(map[string]int, len(adj))
	parent := make(map[string]string, len(adj))

	nodes := make([]string, 0, len(adj))
	for node := range adj {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	for _, node := range nodes {
		if color[node] == colorWhite {
			if cycle, found := dfsVisit(node, adj, color, parent); found {
				return cycle, true
			}
		}
	}
	return nil, false
}

func dfsVisit(node string, adj map[string][]string, color map[string]int, parent map[string]string) ([]string, bool) {
	color[node] = colorGray

	neighbors := slices.Clone(adj[node])
	sort.Strings(neighbors)

	for _, next := range neighbors {
		if color[next] == colorGray {
			return traceCycle(node, next, parent), true
		}
		if color[next] == colorWhite {
			parent[next] = node
			if cycle, found := dfsVisit(next, adj, color, parent); found {
				return cycle, true
			}
		}
	}

	color[node] = colorBlack
	return nil, false
}

func traceCycle(from, to string, parent map[string]string) []string {
	// to is the node we found again (back-edge target), from is the current node.
	// Walk parent chain from `from` back to `to` to reconstruct the cycle.
	var path []string
	cur := from
	for cur != to {
		path = append([]string{cur}, path...)
		cur = parent[cur]
	}
	path = append([]string{to}, path...)
	path = append(path, to)
	return path
}

func formatCyclePath(path []string) string {
	return strings.Join(path, " -> ")
}
