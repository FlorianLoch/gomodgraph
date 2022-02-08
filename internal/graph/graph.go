package graph

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/rs/zerolog/log"

	"github.com/florianloch/gomodgraph/internal/mods"
)

type ModuleNode struct {
	ModuleName   string
	Version      string
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

func (m *ModuleNode) clone() *ModuleNode {
	return &ModuleNode{
		ModuleName:   m.ModuleName,
		Version:      m.Version,
		GoModVersion: m.GoModVersion,
		Requires:     m.Requires,
		RequiredBy:   m.RequiredBy,
		Highlight:    m.Highlight,
	}
}

type DependencyVertex struct {
	targetModule  *ModuleNode
	targetVersion string
}

type DependencyGraph struct {
	// We need a list and a map: the map in order to allow fast lookups,
	// the list in order to render deterministic, reproducible graphs
	// We generate the list from the map as for building the graphs only the map is necessary
	modulesList []*ModuleNode
	modulesMap  map[string]*ModuleNode

	// Indicates whether a graph is a subgraph or the root graph. Only the root graph contains full information,
	// a subgraph's nodes' dependencies are pruned, deriving subgraphs from subgraphs therefore is not a good idea.
	isSubgraph bool
}

func NewDependencyGraph(modulesMap map[string]*ModuleNode, isSubgraph bool) *DependencyGraph {
	modulesList := make([]*ModuleNode, 0, len(modulesMap))

	for _, module := range modulesMap {
		modulesList = append(modulesList, module)
	}

	sort.SliceStable(modulesList, func(i, j int) bool {
		return modulesList[i].ModuleName < modulesList[j].ModuleName
	})

	return &DependencyGraph{
		modulesList: modulesList,
		modulesMap:  modulesMap,
		isSubgraph:  isSubgraph,
	}
}

func BuildDependencyGraph(modFiles []*mods.Module) *DependencyGraph {
	modulesMap := make(map[string]*ModuleNode, len(modFiles))

	// Populate data structures, vertices will be added later-on
	for _, module := range modFiles {
		modFile := module.ModFile

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
			Version:      module.Version,
		}

		modulesMap[moduleName] = moduleNode
	}

	for _, module := range modFiles {
		modFile := module.ModFile

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

	return NewDependencyGraph(modulesMap, false)
}

func (d *DependencyGraph) LookupNode(moduleName string) *ModuleNode {
	return d.modulesMap[moduleName]
}

func (d *DependencyGraph) SubgraphFrom(centerNode *ModuleNode) *DependencyGraph {
	if d.isSubgraph {
		panic("Deriving a subgraph from a subgraph is not recommended")
	}

	subgraphNodesMap := make(map[string]*ModuleNode)

	copiedNode := *centerNode
	centerNode = &copiedNode
	centerNode.Highlight = true

	subgraphNodesMap[centerNode.ModuleName] = centerNode

	// We copy every required module and prune its dependencies
	for _, dependency := range centerNode.Requires {
		cloneModule := dependency.targetModule.clone()
		cloneModule.Requires = nil

		subgraphNodesMap[dependency.targetModule.ModuleName] = cloneModule

		// We also fix this reference because otherwise subgraphNodesMap would not be complete, i.e. there would be
		// references to nodes not contained in the map. The reference would point to a node not contained in the map
		// instead of pointing to the node in the map representing the same module.
		// The rendering implementation depends on the map being complete.
		dependency.targetModule = cloneModule
	}

	// We copy every module requiring the given center module and prune all dependencies, except the one to the center node
	for _, dependency := range centerNode.RequiredBy {
		clonedModule := dependency.targetModule.clone()
		clonedModule.Requires = []*DependencyVertex{{
			targetModule:  centerNode,
			targetVersion: dependency.targetVersion,
		}}

		subgraphNodesMap[dependency.targetModule.ModuleName] = clonedModule
	}

	return NewDependencyGraph(subgraphNodesMap, true)
}

func (d *DependencyGraph) Render(writer io.Writer, goRegistryPrefix string, format graphviz.Format) error {
	g := graphviz.New()

	graph, err := g.Graph()
	if err != nil {
		return fmt.Errorf("instancing Graphviz graph: %w", err)
	}

	// Edge with same target and value can be combined
	graph.SetConcentrate(true)
	graph.SetCenter(true)

	// First, create a lookup map containing all nodes as Graphviz nodes
	graphNodes := make(map[*ModuleNode]*cgraph.Node)

	for _, moduleNode := range d.modulesList {
		n, err := graph.CreateNode(moduleNode.ModuleName)
		if err != nil {
			return fmt.Errorf("creating Graphviz node: %w", err)
		}

		version := moduleNode.Version

		if version == "" {
			version = "<no version yet>"
		}

		n.SetLabel(fmt.Sprintf("%s\n%s (go%s)", strings.TrimPrefix(moduleNode.ModuleName, goRegistryPrefix), version, moduleNode.GoModVersion))

		// We need to fill the node in order to make the whole box a link
		n.SetStyle(cgraph.FilledNodeStyle)

		if moduleNode.Highlight {
			n.SetShape(cgraph.EggShape)
			n.SetColor("crimson")
			n.SetFillColor("goldenrod1")
		} else {
			n.SetURL(fmt.Sprintf("/?mod=%s", url.QueryEscape(moduleNode.ModuleName)))
			n.SetShape(cgraph.BoxShape)
			n.SetFillColor("floralwhite")
		}

		graphNodes[moduleNode] = n
	}

	// Second, connect the nodes
	for _, moduleNode := range d.modulesList {
		for _, dependency := range moduleNode.Requires {
			id := fmt.Sprintf("%s:%s:%s",
				moduleNode.ModuleName,
				dependency.targetModule.ModuleName,
				dependency.targetVersion)

			e, err := graph.CreateEdge(id, graphNodes[moduleNode], graphNodes[dependency.targetModule])
			if err != nil {
				return fmt.Errorf("creating Graphviz edge: %w", err)
			}

			color := "dimgrey"

			// In case the required version is not equal the latest version color the edge differently
			if dependency.targetVersion != dependency.targetModule.Version && dependency.targetModule.Version != "" {
				color = "darkorange"
			}

			e.SetLabel(dependency.targetVersion)
			e.SetColor(color)
			e.SetFontColor(color)
			e.SetArrowSize(0.5)
		}
	}

	// Third, let Graphviz do its magic
	if err := g.Render(graph, format, writer); err != nil {
		return fmt.Errorf("rendering SVG: %w", err)
	}

	return nil
}
