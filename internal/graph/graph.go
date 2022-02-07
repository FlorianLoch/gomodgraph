package graph

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/rs/zerolog/log"
	"golang.org/x/mod/modfile"
)

type ModuleNode struct {
	ModuleName   string
	GoModVersion string
	Requires     []*DependencyVertex
	RequiredBy   []*DependencyVertex
	Highlight    bool
}

func (m *ModuleNode) addDependency(requires *ModuleNode, version string) {
	m.Requires = append(m.Requires, &DependencyVertex{
		targetModule:  requires,
		targetVersion: version,
	})

	requires.RequiredBy = append(requires.RequiredBy, &DependencyVertex{
		targetModule:  m,
		targetVersion: version,
	})
}

type DependencyVertex struct {
	targetModule  *ModuleNode
	targetVersion string
}

type DependencyGraph struct {
	modulesMap map[string]*ModuleNode

	isSubgraph bool // Indicates whether a graph is a subgraph or the root graph. Only the root graph contains full information, a subgraph's nodes dependencies are pruned
}

func BuildDependencyGraph(modFiles []*modfile.File) *DependencyGraph {
	// TODO: Add a list storing the moduleNodes in order tto have deterministic generation of graphs
	modulesMap := make(map[string]*ModuleNode, len(modFiles))

	// Populate data structures, vertices will be added later-on
	for _, modFile := range modFiles {
		if modFile.Module == nil {
			// TODO: Get name of bad module
			log.Error().Msgf("%q does not contain a module directive", "TODO")

			continue
		}

		moduleName := modFile.Module.Mod.Path

		goVersion := "n.a."
		if modFile.Go != nil {
			goVersion = modFile.Go.Version
		}

		moduleNode := &ModuleNode{
			ModuleName:   moduleName,
			GoModVersion: goVersion,
		}

		modulesMap[moduleName] = moduleNode
	}

	for _, modFile := range modFiles {
		if modFile.Module == nil {
			// Issues with this file have already been logged above

			continue
		}

		moduleNode := modulesMap[modFile.Module.Mod.Path]

		// TODO: Handle `Replace` directive

		for _, requiredModule := range modFile.Require {
			if requiredModule.Indirect {
				continue
			}

			requiredModuleNode, ok := modulesMap[requiredModule.Mod.Path]
			if !ok {
				// Not in our set of considered dependencies
				continue
			}

			moduleNode.addDependency(requiredModuleNode, requiredModule.Mod.Version)
		}
	}

	return &DependencyGraph{
		modulesMap: modulesMap,
	}
}

func (d *DependencyGraph) LookupNode(moduleName string) *ModuleNode {
	return d.modulesMap[moduleName]
}

func (d *DependencyGraph) SubgraphFrom(centerNode *ModuleNode) *DependencyGraph {
	subgraphNodesMap := make(map[string]*ModuleNode)

	centerNode = &(*centerNode)
	centerNode.Highlight = true

	subgraphNodesMap[centerNode.ModuleName] = centerNode

	// We copy every required module and prune its dependencies
	for _, dependency := range centerNode.Requires {
		subgraphNodesMap[dependency.targetModule.ModuleName] = &ModuleNode{
			ModuleName:   dependency.targetModule.ModuleName,
			GoModVersion: dependency.targetModule.GoModVersion,
		}
	}

	// We copy every module requiring the given center module and prune all dependencies, except the one to the center node
	for _, dependency := range centerNode.RequiredBy {
		subgraphNodesMap[dependency.targetModule.ModuleName] = &ModuleNode{
			ModuleName:   dependency.targetModule.ModuleName,
			GoModVersion: dependency.targetModule.GoModVersion,
			Requires: []*DependencyVertex{{
				targetModule:  centerNode,
				targetVersion: dependency.targetVersion,
			}},
		}
	}

	return &DependencyGraph{
		modulesMap: subgraphNodesMap,
		isSubgraph: true,
	}
}

func (d *DependencyGraph) RenderSVG(writer io.Writer, goRegistryPrefix string) error {
	g := graphviz.New()

	graph, err := g.Graph()
	if err != nil {
		return fmt.Errorf("instancing Graphviz graph: %w", err)
	}

	// Edge with same target and value can be combined
	graph.SetConcentrate(true)
	graph.SetCenter(true)

	// First, create a lookup map containing all nodes as Graphviz nodes
	graphNodes := make(map[string]*cgraph.Node)

	for moduleName, moduleNode := range d.modulesMap {
		n, err := graph.CreateNode(moduleName)
		if err != nil {
			return fmt.Errorf("creating Graphviz node: %w", err)
		}

		n.SetLabel(fmt.Sprintf("%s\n(%s)", strings.TrimPrefix(moduleName, goRegistryPrefix), moduleNode.GoModVersion))

		// We need to fill the node in order to make the whole box a link
		n.SetStyle(cgraph.FilledNodeStyle)
		n.SetFillColor("white")

		if moduleNode.Highlight {
			n.SetColor("crimson")
			n.SetFontColor("crimson")
			n.SetShape(cgraph.OctagonShape)
		} else {
			n.SetURL(fmt.Sprintf("/?mod=%s", url.QueryEscape(moduleName)))
			n.SetShape(cgraph.BoxShape)
		}

		graphNodes[moduleName] = n
	}

	// Second, connect the nodes
	for moduleName, moduleNode := range d.modulesMap {
		for _, dependency := range moduleNode.Requires {
			id := fmt.Sprintf("%s:%s:%s",
				moduleName,
				dependency.targetModule.ModuleName,
				dependency.targetVersion)

			e, err := graph.CreateEdge(id, graphNodes[moduleName], graphNodes[dependency.targetModule.ModuleName])
			if err != nil {
				return fmt.Errorf("creating Graphviz edge: %w", err)
			}

			e.SetLabel(dependency.targetVersion)
			e.SetColor("dimgrey")
			e.SetFontColor("dimgrey")
			e.SetArrowSize(0.5)
		}
	}

	// Third, let Graphviz do its magic
	if err := g.Render(graph, graphviz.SVG, writer); err != nil {
		return fmt.Errorf("rendering SVG: %w", err)
	}

	return nil
}
