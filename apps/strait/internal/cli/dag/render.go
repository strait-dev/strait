// Package dag provides workflow DAG visualization using box-drawing Unicode characters.
package dag

import (
	"fmt"
	"sort"
	"strings"
)

// Step represents a single step in a workflow DAG.
type Step struct {
	StepRef   string
	DependsOn []string
}

// statusColor returns an ANSI color code prefix for the given status.
func statusColor(status string) string {
	switch status {
	case "completed":
		return "\033[32m" // green
	case "failed", "crashed", "system_failed":
		return "\033[31m" // red
	case "running", "executing":
		return "\033[33m" // yellow
	case "pending", "waiting":
		return "\033[34m" // blue
	case "skipped", "canceled":
		return "\033[90m" // gray
	default:
		return ""
	}
}

const resetColor = "\033[0m"

// RenderDAG produces a text-based DAG visualization using box-drawing characters.
// statusMap maps step_ref to a status string for color-coding. Pass nil to skip coloring.
func RenderDAG(steps []Step, statusMap map[string]string) string {
	if len(steps) == 0 {
		return "(empty workflow)"
	}

	// Build adjacency and compute topological order via Kahn's algorithm
	refs := make(map[string]bool)
	children := make(map[string][]string) // parent -> children
	inDegree := make(map[string]int)

	for _, s := range steps {
		refs[s.StepRef] = true
		if _, ok := inDegree[s.StepRef]; !ok {
			inDegree[s.StepRef] = 0
		}
	}

	for _, s := range steps {
		for _, dep := range s.DependsOn {
			children[dep] = append(children[dep], s.StepRef)
			inDegree[s.StepRef]++
		}
	}

	// Kahn's algorithm
	queue := make([]string, 0)
	for ref := range refs {
		if inDegree[ref] == 0 {
			queue = append(queue, ref)
		}
	}
	sort.Strings(queue) // deterministic ordering

	layers := make([][]string, 0)
	for len(queue) > 0 {
		layer := make([]string, len(queue))
		copy(layer, queue)
		sort.Strings(layer)
		layers = append(layers, layer)

		next := make([]string, 0)
		for _, node := range queue {
			for _, child := range children[node] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}
		sort.Strings(next)
		queue = next
	}

	// Check for cycle (unreachable nodes)
	visited := 0
	for _, layer := range layers {
		visited += len(layer)
	}
	if visited < len(refs) {
		return "(cycle detected in workflow DAG)"
	}

	// Build deps map for annotation
	depsMap := make(map[string][]string)
	for _, s := range steps {
		depsMap[s.StepRef] = s.DependsOn
	}

	// Render
	var buf strings.Builder
	for i, layer := range layers {
		if i > 0 {
			// Draw connectors from previous layer
			indent := "  "
			if len(layer) == 1 {
				buf.WriteString(indent)
				buf.WriteString("  |")
				buf.WriteString("\n")
				buf.WriteString(indent)
				buf.WriteString("  v")
				buf.WriteString("\n")
			} else {
				// Multiple nodes: fan-out
				buf.WriteString(indent)
				for j := range layer {
					if j == 0 {
						buf.WriteString("  |")
					} else {
						buf.WriteString("---+")
					}
				}
				buf.WriteString("\n")
				buf.WriteString(indent)
				for j := range layer {
					if j == 0 {
						buf.WriteString("  v")
					} else {
						buf.WriteString("   v")
					}
				}
				buf.WriteString("\n")
			}
		}

		// Render nodes in this layer
		for j, ref := range layer {
			if j > 0 {
				buf.WriteString("  ")
			}
			renderNode(&buf, ref, statusMap, depsMap)
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

func renderNode(buf *strings.Builder, ref string, statusMap map[string]string, depsMap map[string][]string) {
	status := ""
	if statusMap != nil {
		status = statusMap[ref]
	}

	label := ref
	if status != "" {
		label = fmt.Sprintf("%s [%s]", ref, status)
	}

	color := statusColor(status)
	if color != "" {
		buf.WriteString(color)
	}

	// Box drawing
	width := len(label) + 2
	top := strings.Repeat("\u2500", width)
	buf.WriteString("\u250c" + top + "\u2510")

	if color != "" {
		buf.WriteString(resetColor)
	}

	buf.WriteString("\n")

	if color != "" {
		buf.WriteString(color)
	}
	buf.WriteString("\u2502 " + label + " \u2502")
	if color != "" {
		buf.WriteString(resetColor)
	}

	// Show deps as annotation
	if deps, ok := depsMap[ref]; ok && len(deps) > 0 {
		fmt.Fprintf(buf, "  <- %s", strings.Join(deps, ", "))
	}
	buf.WriteString("\n")

	if color != "" {
		buf.WriteString(color)
	}
	buf.WriteString("\u2514" + top + "\u2518")
	if color != "" {
		buf.WriteString(resetColor)
	}
}
