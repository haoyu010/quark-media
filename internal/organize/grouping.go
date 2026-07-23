//go:build ignore
// +build ignore

package organize

import (
	"path/filepath"
	"sort"
)

// ResourceGroup represents a set of media files belonging to the same resource
// (e.g. a TV show or movie) detected by directory-tree branch analysis.
type ResourceGroup struct {
	ResourceName string   // leaf directory name at the resource boundary
	Paths        []string // relative paths of all media files in this group
}

// GroupByResourceBoundary groups media file paths by dynamically detecting
// resource boundaries in the directory tree.
//
// Algorithm: walk up from each file's leaf directory until a parent with
// multiple media-bearing children is found — that child is the resource
// boundary. This handles summary folders, Season directories, and
// nested collection hierarchies without relying on fixed depth levels.
//
// Examples:
//
//	["漫威/钢铁侠1/a.mkv", "漫威/钢铁侠2/b.mkv"]  → 2 groups (钢铁侠1, 钢铁侠2)
//	["剧/S01/E01.mkv", "剧/S02/E01.mkv"]           → 1 group  (剧)
func GroupByResourceBoundary(paths []string) []ResourceGroup {
	if len(paths) == 0 {
		return nil
	}

	// Step 1: build directory tree metadata.
	dirMediaCount := map[string]int{}           // dir → number of descendant media files
	dirChildren := map[string]map[string]bool{} // parent → set of child dirs that contain media
	fileLeafDir := make([]string, len(paths))   // file index → its direct parent directory

	for i, p := range paths {
		leafDir := filepath.Dir(filepath.ToSlash(p))
		if leafDir == "." {
			leafDir = ""
		}
		fileLeafDir[i] = leafDir

		// Count this file in every ancestor directory.
		for d := leafDir; d != ""; d = parentDir(d) {
			dirMediaCount[d]++
		}

		// Build parent→child relationships for each ancestor.
		for d := leafDir; d != ""; {
			p := parentDir(d)
			if p == "" || p == d {
				break
			}
			if dirChildren[p] == nil {
				dirChildren[p] = map[string]bool{}
			}
			dirChildren[p][d] = true
			d = p
		}
	}

	// Step 2: find resource boundary for each file.
	resourceFiles := map[string][]int{} // resourceDir → file indexes
	for i := range paths {
		leaf := fileLeafDir[i]
		resourceDir := findResourceBoundary(leaf, dirMediaCount, dirChildren)
		resourceFiles[resourceDir] = append(resourceFiles[resourceDir], i)
	}

	// Step 3: assemble results.
	groups := make([]ResourceGroup, 0, len(resourceFiles))
	for resDir, indexes := range resourceFiles {
		name := filepath.Base(resDir)
		if name == "." || name == "" {
			// Files at root level: use first file's stem as fallback name.
			if len(indexes) > 0 {
				name = cleanRootFileResourceName(filepath.Base(paths[indexes[0]]))
			}
		}
		groupPaths := make([]string, 0, len(indexes))
		for _, idx := range indexes {
			groupPaths = append(groupPaths, paths[idx])
		}
		sort.Strings(groupPaths)
		groups = append(groups, ResourceGroup{
			ResourceName: name,
			Paths:        groupPaths,
		})
	}

	// Sort groups by resource name for deterministic output.
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].ResourceName < groups[j].ResourceName
	})

	return groups
}

// findResourceBoundary walks up from leafDir to find the resource boundary:
// the deepest directory whose parent has more than one media-bearing child.
func findResourceBoundary(
	leafDir string,
	dirMediaCount map[string]int,
	dirChildren map[string]map[string]bool,
) string {
	if leafDir == "" {
		return ""
	}

	resourceDir := leafDir
	for dir := leafDir; dir != ""; {
		p := parentDir(dir)
		if p == "" || p == dir {
			// Reached root: dir is the resource boundary.
			resourceDir = dir
			break
		}
		// Count how many children of parent contain media files.
		mediaChildren := 0
		for child := range dirChildren[p] {
			if dirMediaCount[child] > 0 {
				mediaChildren++
			}
		}
		if mediaChildren > 1 {
			resourceDir = dir
			break
		}
		// Parent has only 1 media-bearing child — keep walking up.
		dir = p
	}
	return resourceDir
}

// parentDir returns the parent directory of path in slash-normalized form.
// Returns "" when path is at the root level.
func parentDir(path string) string {
	path = filepath.ToSlash(path)
	dir := filepath.Dir(path)
	if dir == "." || dir == path {
		return ""
	}
	return filepath.ToSlash(dir)
}
