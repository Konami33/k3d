package run

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/go-connections/nat"
)

// PublishedPorts is a struct used for exposing container ports on the host system
type PublishedPorts struct {
	ExposedPorts map[nat.Port]struct{}
	PortBindings map[nat.Port][]nat.PortBinding
}

// defaultNodes describes the type of nodes on which a port should be exposed by default
const defaultNodes = "all"

// mapping a node role to groups that should be applied to it
var nodeRuleGroupsMap = map[string][]string{
	"worker": []string{"all", "workers"},
	"server": []string{"all", "server", "master"},
}

// mapNodesToPortSpecs maps nodes to portSpecs
func mapNodesToPortSpecs(specs []string) (map[string][]string, error) {

	if err := validatePortSpecs(specs); err != nil {
		return nil, err
	}

	nodeToPortSpecMap := make(map[string][]string)

	for _, spec := range specs {
		// extractNodes returns a list of nodes and the port specification
		nodes, portSpec := extractNodes(spec)

		for _, node := range nodes {
			// each node is mapped to a slice of port specifications.
			nodeToPortSpecMap[node] = append(nodeToPortSpecMap[node], portSpec)
		}
	}
	fmt.Printf("nodeToPortSpecMap: %+v\n", nodeToPortSpecMap)

	return nodeToPortSpecMap, nil
}

// The factory function for PublishedPorts
// createPublishedPorts creates a PublishedPorts object from a list of port specifications
func CreatePublishedPorts(specs []string) (*PublishedPorts, error) {
	// if no specs defined create an empty PublishedPorts object
	if len(specs) == 0 {
		var newExposedPorts = make(map[nat.Port]struct{}, 1)
		var newPortBindings = make(map[nat.Port][]nat.PortBinding, 1)
		return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}, nil
	}

	// ParsePortSpecs receives port specs in the format of ip:public:private/proto and parses these in to the internal types
	// (map[nat.Port]struct{}, map[nat.Port][]nat.PortBinding, error)
	newExposedPorts, newPortBindings, err := nat.ParsePortSpecs(specs)
	return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}, err
}

// validatePortSpecs matches the provided port specs against a set of rules to enable early exit if something is wrong
// specs represents the port mapping specifications
//((?P<host>[\w\.]+)?:)?: This part matches an optional hostname followed by a colon. It captures the hostname in the group named host.
// ((?P<hostPort>[0-9]{0,6}):)?: This part matches an optional host port followed by a colon. It captures the host port in the group named hostPort.
// (?P<containerPort>[0-9]{1,6}): This part matches the container port. It captures the container port in the group named containerPort.
// ((/(?P<protocol>udp|tcp))?(?P<nodes>(@(?P<node>[\w-]+))*)): This part matches the protocol (either udp or tcp) and any associated nodes. It captures the protocol in the group named protocol and the nodes in the group named nodes.

func validatePortSpecs(specs []string) error {
	// regex matching (no sophisticated IP/Hostname matching at the moment)
	regex := regexp.MustCompile(`^(((?P<host>[\w\.]+)?:)?((?P<hostPort>[0-9]{0,6}):)?(?P<containerPort>[0-9]{1,6}))((/(?P<protocol>udp|tcp))?(?P<nodes>(@(?P<node>[\w-]+))*))$`)
	for _, spec := range specs {
		if !regex.MatchString(spec) {
			return fmt.Errorf("[ERROR] Provided port spec [%s] didn't match format specification", spec)
		}
	}
	return nil
}

// extractNodes separates the node specification from the actual port specse
// use case:
// Suppose spec is "80:80@node1@node2".
// After splitting, portSpec becomes "80:80", and nodes becomes ["node1", "node2"].
// If spec were "80:80", portSpec would still be "80:80", but nodes would be ["all"] because no specific nodes were provided.
func extractNodes(spec string) ([]string, string) {
	// extract nodes
	nodes := []string{}
	// Split slices s :="spec" into all substrings separated by sep(here @) and returns a slice of the substrings between those separators.
	atSplit := strings.Split(spec, "@")
	// atSplit[0] is the port spec
	portSpec := atSplit[0]
	if len(atSplit) > 1 {
		nodes = atSplit[1:]
	}
	// if no nodes are specified, use the default = "all"
	if len(nodes) == 0 {
		nodes = append(nodes, defaultNodes)
	}
	return nodes, portSpec
}

// Offset creates a new PublishedPort structure, with all host ports are changed by a fixed  'offset'
func (p PublishedPorts) Offset(offset int) *PublishedPorts {
	var newExposedPorts = make(map[nat.Port]struct{}, len(p.ExposedPorts))
	var newPortBindings = make(map[nat.Port][]nat.PortBinding, len(p.PortBindings))

	for k, v := range p.ExposedPorts {
		newExposedPorts[k] = v
	}

	for k, v := range p.PortBindings {

		bindings := make([]nat.PortBinding, len(v))
		for i, b := range v {
			port, _ := nat.ParsePort(b.HostPort)
			bindings[i].HostIP = b.HostIP
			bindings[i].HostPort = fmt.Sprintf("%d", port+offset)
		}
		newPortBindings[k] = bindings
	}

	return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}
}

// AddPort creates a new PublishedPort struct with one more port, based on 'portSpec'
func (p *PublishedPorts) AddPort(portSpec string) (*PublishedPorts, error) {
	portMappings, err := nat.ParsePortSpec(portSpec)
	if err != nil {
		return nil, err
	}

	var newExposedPorts = make(map[nat.Port]struct{}, len(p.ExposedPorts)+1)
	var newPortBindings = make(map[nat.Port][]nat.PortBinding, len(p.PortBindings)+1)

	// Populate the new maps
	for k, v := range p.ExposedPorts {
		newExposedPorts[k] = v
	}

	for k, v := range p.PortBindings {
		newPortBindings[k] = v
	}

	// Add new ports
	for _, portMapping := range portMappings {
		port := portMapping.Port
		if _, exists := newExposedPorts[port]; !exists {
			newExposedPorts[port] = struct{}{}
		}

		bslice, exists := newPortBindings[port]
		if !exists {
			bslice = []nat.PortBinding{}
		}
		newPortBindings[port] = append(bslice, portMapping.Binding)
	}

	return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}, nil
}

// MergePortSpecs merges published ports for a given node
func MergePortSpecs(nodeToPortSpecMap map[string][]string, role, name string) ([]string, error) {

	portSpecs := []string{}

	// add portSpecs according to node role: server or worker
	for _, group := range nodeRuleGroupsMap[role] {
		for _, v := range nodeToPortSpecMap[group] {
			exists := false
			for _, i := range portSpecs {
				if v == i {
					exists = true
				}
			}
			if !exists {
				portSpecs = append(portSpecs, v)
			}
		}
	}

	// add portSpecs according to node name
	for _, v := range nodeToPortSpecMap[name] {
		exists := false
		for _, i := range portSpecs {
			if v == i {
				exists = true
			}
		}
		if !exists {
			portSpecs = append(portSpecs, v)
		}
	}
	return portSpecs, nil
}
