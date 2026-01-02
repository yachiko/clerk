package ui

import (
	"sort"
	"strings"

	"github.com/yachiko/clerk/internal/cache"
)

// dirInfo is a directory structure for building tree
type dirInfo struct {
	children map[string]*dirInfo
	entries  []*cache.CacheEntry
}

// buildTreeNodes builds a tree structure from cache entries
func buildTreeNodes(entries []cache.CacheEntry, expanded map[string]bool) []TreeNode {
	if len(entries) == 0 {
		return nil
	}

	root := &dirInfo{children: make(map[string]*dirInfo)}

	for i := range entries {
		entry := &entries[i]
		parts := strings.Split(strings.TrimPrefix(entry.Name, "/"), "/")

		current := root
		for j, part := range parts {
			if j == len(parts)-1 {
				// This is the leaf (actual parameter)
				current.entries = append(current.entries, entry)
			} else {
				// This is a directory
				if current.children[part] == nil {
					current.children[part] = &dirInfo{children: make(map[string]*dirInfo)}
				}
				current = current.children[part]
			}
		}
	}

	// Flatten to nodes
	var nodes []TreeNode
	buildNodesRecursive(root, "", 0, expanded, &nodes)

	return nodes
}

func buildNodesRecursive(dir *dirInfo, path string, depth int, expanded map[string]bool, nodes *[]TreeNode) {
	// Sort directory names
	dirNames := make([]string, 0, len(dir.children))
	for name := range dir.children {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)

	// Sort entry names
	sort.Slice(dir.entries, func(i, j int) bool {
		return dir.entries[i].Name < dir.entries[j].Name
	})

	// Process directories first
	for _, name := range dirNames {
		child := dir.children[name]
		childPath := path + "/" + name

		isExpanded := expanded[childPath]
		childCount := countEntries(child)

		*nodes = append(*nodes, TreeNode{
			Name:       name,
			Path:       childPath,
			IsDir:      true,
			Depth:      depth,
			Expanded:   isExpanded,
			ChildCount: childCount,
		})

		if isExpanded {
			buildNodesRecursive(child, childPath, depth+1, expanded, nodes)
		}
	}

	// Process entries
	for _, entry := range dir.entries {
		// Get just the name part
		parts := strings.Split(entry.Name, "/")
		name := parts[len(parts)-1]

		*nodes = append(*nodes, TreeNode{
			Name:  name,
			Path:  entry.Name,
			IsDir: false,
			Depth: depth,
			Entry: entry,
		})
	}
}

func countEntries(dir *dirInfo) int {
	count := len(dir.entries)
	for _, child := range dir.children {
		count += countEntries(child)
	}
	return count
}
