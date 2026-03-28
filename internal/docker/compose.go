package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// LoadDependencyOrder parses a compose file and returns service names in
// dependency order (dependencies first). Returns an error if a cycle is detected.
func LoadDependencyOrder(composePath string) ([]string, error) {
	absPath, err := filepath.Abs(composePath)
	if err != nil {
		return nil, fmt.Errorf("resolving compose path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading compose file: %w", err)
	}

	project, err := loader.LoadWithContext(context.Background(), types.ConfigDetails{
		WorkingDir: filepath.Dir(absPath),
		ConfigFiles: []types.ConfigFile{
			{
				Filename: absPath,
				Content:  content,
			},
		},
		Environment: types.Mapping{},
	}, loader.WithSkipValidation)
	if err != nil {
		return nil, fmt.Errorf("parsing compose file: %w", err)
	}

	// Build adjacency list and collect all service names
	deps := make(map[string][]string)
	var allNodes []string
	for name, svc := range project.Services {
		allNodes = append(allNodes, name)
		for dep := range svc.DependsOn {
			deps[name] = append(deps[name], dep)
		}
	}

	return topoSort(deps, allNodes)
}

// FilterDependencyOrder returns only the target service names from the full
// dependency order, preserving their relative order.
func FilterDependencyOrder(allOrder []string, targets []string) []string {
	targetSet := make(map[string]bool, len(targets))
	for _, t := range targets {
		targetSet[t] = true
	}

	var filtered []string
	for _, name := range allOrder {
		if targetSet[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

// topoSort performs a topological sort using Kahn's algorithm.
// deps maps each node to its dependencies (edges point from node to dependency).
// Returns nodes in dependency-first order, or an error if a cycle is detected.
func topoSort(deps map[string][]string, allNodes []string) ([]string, error) {
	// Build in-degree map (how many things depend on each node is irrelevant;
	// we need: for each node, how many dependencies does it have)
	// Actually for Kahn's, we need the reverse: in-degree = number of incoming edges
	// where an edge goes from dependency to dependent.
	//
	// deps[A] = [B, C] means A depends on B and C.
	// In the DAG: B -> A, C -> A (edges from dep to dependent).
	// So in-degree of A = len(deps[A]).

	// Ensure all nodes are in the map
	nodeSet := make(map[string]bool, len(allNodes))
	for _, n := range allNodes {
		nodeSet[n] = true
	}

	// in-degree: number of dependencies each node has
	inDegree := make(map[string]int, len(allNodes))
	for _, n := range allNodes {
		inDegree[n] = len(deps[n])
	}

	// reverse adjacency: dep -> list of dependents (for decrementing in-degree)
	reverse := make(map[string][]string)
	for node, nodeDeps := range deps {
		for _, d := range nodeDeps {
			reverse[d] = append(reverse[d], node)
		}
	}

	// Start with nodes that have zero dependencies
	var queue []string
	for _, n := range allNodes {
		if inDegree[n] == 0 {
			queue = append(queue, n)
		}
	}

	// Sort queue for deterministic output
	sortStrings(queue)

	var result []string
	for len(queue) > 0 {
		// Pop first
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// Decrement in-degree for all dependents
		for _, dependent := range reverse[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = insertSorted(queue, dependent)
			}
		}
	}

	if len(result) != len(allNodes) {
		return nil, fmt.Errorf("dependency cycle detected: processed %d of %d services", len(result), len(allNodes))
	}

	return result, nil
}

// sortStrings sorts a string slice in place (simple insertion sort for small slices).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// insertSorted inserts a string into a sorted slice maintaining sorted order.
func insertSorted(s []string, item string) []string {
	i := 0
	for i < len(s) && s[i] < item {
		i++
	}
	s = append(s, "")
	copy(s[i+1:], s[i:])
	s[i] = item
	return s
}
