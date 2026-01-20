package domain

import (
	"sort"
	"strings"
)

// BuildDirectoryTreeFromFiles builds an ASCII directory tree from the included file list.
// This is deterministic and matches scanning rules (since it only uses included files).
//
// Depth semantics: maxDepth counts path segments from root (1 = direct children).
func BuildDirectoryTreeFromFiles(rootName string, relPaths []string, maxDepth int) string {
	if maxDepth < 1 {
		maxDepth = 1
	}
	if strings.TrimSpace(rootName) == "" {
		rootName = "."
	}

	// Trie
	type node struct {
		name     string
		children map[string]*node
		isFile   bool
	}

	root := &node{name: rootName, children: map[string]*node{}}

	insert := func(rel string) {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			return
		}
		parts := strings.Split(rel, "/")
		cur := root
		for idx, p := range parts {
			if p == "" {
				continue
			}
			if cur.children == nil {
				cur.children = map[string]*node{}
			}
			child := cur.children[p]
			if child == nil {
				child = &node{name: p, children: map[string]*node{}}
				cur.children[p] = child
			}
			cur = child
			if idx == len(parts)-1 {
				cur.isFile = true
			}
		}
	}

	for _, p := range relPaths {
		insert(p)
	}

	var lines []string
	lines = append(lines, rootName)

	var walk func(n *node, depth int, prefix string)
	walk = func(n *node, depth int, prefix string) {
		if depth > maxDepth {
			return
		}
		if n.children == nil || len(n.children) == 0 {
			return
		}

		// Sort children: directories first, then files, then name.
		names := make([]string, 0, len(n.children))
		for k := range n.children {
			names = append(names, k)
		}
		sort.Slice(names, func(i, j int) bool {
			ni := n.children[names[i]]
			nj := n.children[names[j]]
			iIsDir := ni != nil && len(ni.children) > 0 && !ni.isFile
			jIsDir := nj != nil && len(nj.children) > 0 && !nj.isFile
			if iIsDir != jIsDir {
				return iIsDir
			}
			return names[i] < names[j]
		})

		for idx, name := range names {
			child := n.children[name]
			if child == nil {
				continue
			}
			isLast := idx == len(names)-1
			branch := "├── "
			nextPrefix := prefix + "│   "
			if isLast {
				branch = "└── "
				nextPrefix = prefix + "    "
			}

			lines = append(lines, prefix+branch+child.name)

			// Depth is segment-depth from root line.
			if depth < maxDepth {
				walk(child, depth+1, nextPrefix)
			}
		}
	}

	walk(root, 1, "")
	return strings.Join(lines, "\n")
}
