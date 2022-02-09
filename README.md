# gomodgraph

Purpose of this tool is to visualize the dependencies between a given bunch of Go modules.
Currently, there only is support to retrieve such a set of modules from on-premise GitLab instances.

This may not be helpful for projects with external dependencies only, but quite handy for corporate architectures with many Go modules depending on each other.

![Example of graph generated with `gomodgraph`](./example_graph.svg "example")

`gomodgraph` runs locally.
Initially it will fetch available modules and version information from GitLab.
It will check all projects accessible with the given token for top-level `go.mod` files.
The results will be cached until run with `-cleanup`.

Afterwards it will start an HTTP server, serving the "overview" diagram by default.
This can be configured using CLI flags.
The overview might not be very helpful as the underlying, quite awesome, graphviz does not really know how to render this in a nice way.
Within the graph, nodes represent modules, edged dependencies.
Orange edges indicate a dependencies referring to a version not being the latest version of the required module.

**The graph is interactive, one can drill down on specific modules by clicking on the respective node.**

Adding `png` as query parameter will return a PNG instead of an SVG - but the quality of the rendering is quite poor so taking a screenshot or converting the SVG manually is recommended.

**This project is still in an early development phase and might not work perfectly/at all.**

# Install
```shell
go install github.com/florianloch/gomodgraph@latest
```

# Run
```
# All projects visible to the user bound to this token will be considered
export GITLAB_API_TOKEN = "your token"

# Can also be set as CLI flag
export GITLAB_BASE_URL = "base url"

gomodgraph

# Starting with a specific module straight away
gomodgraph -mod your-go-registry.com/module_A
```