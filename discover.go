package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func discoverOutputs(paths []string, depth int) ([]string, map[string]string) {
	seen := make(map[string]bool)
	roots := make(map[string]string)
	var results []string

	for _, input := range paths {
		input = filepath.Clean(input)

		if filepath.Base(input) == "outputs" {
			if !seen[input] {
				seen[input] = true
				results = append(results, input)
			}
			continue
		}

		childOutputs := filepath.Join(input, "outputs")
		if info, err := os.Stat(childOutputs); err == nil && info.IsDir() {
			if !seen[childOutputs] {
				seen[childOutputs] = true
				results = append(results, input)
			}
			continue
		}

		for _, d := range discoverUnder(input, depth) {
			if !seen[d] {
				seen[d] = true
				results = append(results, d)
				roots[d] = input
			}
		}
	}

	return results, roots
}

func discoverUnder(root string, maxDepth int) []string {
	if maxDepth <= 0 {
		return nil
	}

	var results []string

	type queueItem struct {
		path  string
		depth int
	}
	queue := []queueItem{{root, 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		entries, err := os.ReadDir(item.path)
		if err != nil {
			continue
		}

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()

			if strings.HasPrefix(name, ".") {
				continue
			}
			if name == "zarr" || strings.HasSuffix(name, ".zarr") {
				continue
			}

			fullPath := filepath.Join(item.path, name)

			if name == "outputs" {
				results = append(results, fullPath)
				continue
			}

			if item.depth+1 < maxDepth {
				queue = append(queue, queueItem{fullPath, item.depth + 1})
			}
		}
	}

	return results
}
