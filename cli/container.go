package run

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerClient "github.com/docker/docker/client"
)

// PublishedPorts is a struct that represents the ports exposed by a container along with their bindings.
// ExposedPorts: keys --> exposed ports, values --> empty structs.
// PortBindings: keys --> exposed ports, values --> slices of nat.PortBinding structs
type PublishedPorts struct {
	ExposedPorts map[nat.Port]struct{}
	PortBindings map[nat.Port][]nat.PortBinding // representing port bindings.
}

// The factory function for PublishedPorts
func createPublishedPorts(specs []string) (*PublishedPorts, error) {
	//no specs defined. so create empty ports and bindings
	if len(specs) == 0 {
		//Essentially, this line creates an empty map capable of storing nat.Port keys. Second argument specifies the initial capacity of 1.
		var newExposedPorts = make(map[nat.Port]struct{}, 1)
		var newPortBindings = make(map[nat.Port][]nat.PortBinding, 1)
		return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}, nil
	}

	// specs: each string represents a port specification in the format ip:public:private/proto.
	// the values are slices of nat.PortBinding structs. Each nat.PortBinding struct likely contains information about the host IP and port to which the exposed port is bound.
	newExposedPorts, newPortBindings, err := nat.ParsePortSpecs(specs)
	return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}, err
}

// Create a new PublishedPort structure, with all host ports are changed by a fixed  'offset'
func (p PublishedPorts) Offset(offset int) *PublishedPorts {
	// initializes a new map with size len of p.ExposedPorts map
	var newExposedPorts = make(map[nat.Port]struct{}, len(p.ExposedPorts))
	var newPortBindings = make(map[nat.Port][]nat.PortBinding, len(p.PortBindings))

	//copy
	for k, v := range p.ExposedPorts {
		newExposedPorts[k] = v
	}

	//copy
	// iterates over the PortBindings map of the PublishedPorts structure (p).
	// In each iteration,
	// k is the key --> exposed port,
	// v is the value --> slice of nat.PortBinding structs representing the port bindings).
	for k, v := range p.PortBindings {
		// PortBinding represents a binding between a Host IP address and a Host Port
		// a new slice of type nat.PortBinding to store the modified port bindings.
		bindings := make([]nat.PortBinding, len(v))
		for i, b := range v {
			// iterates of each port binding 'b'  within the slice of port bindings (v).
			// i is the index of the current port binding in the slice
			//ParsePort parses the port number string and returns an int
			port, _ := nat.ParsePort(b.HostPort)
			bindings[i].HostIP = b.HostIP
			bindings[i].HostPort = fmt.Sprintf("%d", port+offset)
		}
		newPortBindings[k] = bindings
	}

	return &PublishedPorts{ExposedPorts: newExposedPorts, PortBindings: newPortBindings}
}

// Create a new PublishedPort struct with one more port, based on 'portSpec'
func (p *PublishedPorts) AddPort(portSpec string) (*PublishedPorts, error) {
	// Parses the port specification string (portSpec) into a slice of port mappings. 
	// Each port mapping consists of a port --> corresponding binding.
	// portSpec: "0.0.0.0:%s:%s/tcp", port, port, here port is 6443
	// after parsing:
	// portMappings := []nat.PortBinding{
	// 	{
	// 		HostIP:        "0.0.0.0",
	// 		HostPort:      "6443",
	// 		ContainerPort: "6443",
	// 		Protocol:      "tcp",
	// 	},
	// }
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
	// var portMappings []nat.PortMapping.is a slice of port mappings.
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

// To do: update this function and solve why the package function is not working
//
//	func IsImageNotFoundError(err error) bool {
//		// Check if the error message contains a string indicating that the image is not found
//		return strings.Contains(err.Error(), "No such image") || strings.Contains(err.Error(), "not found")
//	}
func startContainer(verbose bool, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (string, error) {
	ctx := context.Background()

	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	// first try createContainer by assuming the image is locally available
	// resp --> container create response. An object representing the response from Docker after creating the container. It contains information about the newly created container, such as its unique identifier (ID).

	log.Printf("Pulling image %s...\n", config.Image)
	// var reader io.ReadCloser. ImagePull function returns (io.ReadCloser, error)
	reader, err := docker.ImagePull(ctx, config.Image, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't pull image %s\n%+v", config.Image, err)
	}
	// It's up to the caller to handle the reader (io.ReadCloser) and close it properly.
	defer reader.Close()
	if verbose {
		// Copy copies from src to dst until either EOF is reached on src or an error occurs. It returns the number of bytes copied and the first error encountered while copying,
		_, err := io.Copy(os.Stdout, reader)
		if err != nil {
			log.Printf("WARNING: couldn't get docker output\n%+v", err)
		}
	} else {
		_, err := io.Copy(io.Discard, reader)
		if err != nil {
			log.Printf("WARNING: couldn't get docker output\n%+v", err)
		}
	}
	// after pulling the image try containerCreate again
	resp, err := docker.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create container after pull %s\n%+v", containerName, err)
	}

	// start the container
	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func createServer(verbose bool, image string, port string, args []string, env []string, name string, volumes []string, pPorts *PublishedPorts) (string, error) {
	log.Printf("Creating server using %s...\n", image)

	containerLabels := make(map[string]string)
	containerLabels["app"] = "k3d"
	containerLabels["component"] = "server"
	containerLabels["created"] = time.Now().Format("2006-01-02 15:04:05")
	containerLabels["cluster"] = name

	containerName := fmt.Sprintf("k3d-%s-server", name)

	//containerPort := nat.Port(fmt.Sprintf("%s/tcp", port))
	//problem
	apiPortSpec := fmt.Sprintf("0.0.0.0:%s:%s/tcp", port, port)
	serverPublishedPorts, err := pPorts.AddPort(apiPortSpec)
	if (err != nil) {
		log.Fatalf("Error: failed to parse API port spec %s \n%+v", apiPortSpec, err)
	}

	//handle hostconfig
	hostConfig := &container.HostConfig{
		// Port mapping between the exposed port (container) and the host
		// Key = containerPort. Represents the port inside the container
		// Value = []nat.PortBinding. Represents the port on the host machine. Each nat.PortBinding struct specifies the mapping of a container port to a host port.
		PortBindings: serverPublishedPorts.PortBindings,
		Privileged: true,
	}

	//handle volume
	if len(volumes) > 0 && volumes[0] != "" {
		hostConfig.Binds = volumes
	}

	//networkingConfig
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			name: {
				Aliases: []string{containerName},
			},
		},
	}

	// Config contains the configuration data about a container. It should hold only portable information about the container. Here, "portable" means "independent from the host we are running on"
	config := &container.Config{
		Hostname: containerName,
		Image:    image,
		Cmd:      append([]string{"server"}, args...),
		ExposedPorts: serverPublishedPorts.ExposedPorts,
		Env:    env,
		Labels: containerLabels,
	}
	// image format
	fmt.Println(config.Image)
	//contianer creattion response ie resp.ID
	id, err := startContainer(verbose, config, hostConfig, networkingConfig, containerName)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create container %s\n%+v", containerName, err)
	}

	return id, nil
}

// creating worker node
func createWorker(verbose bool, image string, args []string, env []string, name string, volumes []string, postfix string, serverPort string) (string, error) {

	//create the container basic info
	containerLabels := make(map[string]string)
	containerLabels["app"] = "k3d"
	containerLabels["component"] = "worker"
	containerLabels["created"] = time.Now().Format("2006-01-02 15:04:05")
	containerLabels["cluster"] = name

	containerName := fmt.Sprintf("k3d-%s-worker-%s", name, postfix)

	env = append(env, fmt.Sprintf("K3S_URL=https://k3d-%s-server:%s", name, serverPort))

	hostConfig := &container.HostConfig{
		//  Each entry represents a temporary filesystem (tmpfs) mount point within the container.
		// Tmpfs is a filesystem that resides in memory and is mounted as a virtual filesystem.
		//keys --> representing the mount points means directories
		//values --> representing mount options. for this case empty
		// By mounting them as tmpfs, any data written to these directories within the container will be stored in memory rather than on disk.
		Tmpfs: map[string]string{
			"/run":     "",
			"/var/run": "",
		},
		Privileged: true,
	}

	if len(volumes) > 0 && volumes[0] != "" {
		hostConfig.Binds = volumes
	}

	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			name: {
				Aliases: []string{containerName},
			},
		},
	}

	config := &container.Config{
		Hostname: containerName,
		Image:    image,
		Env:      env,
		Labels:   containerLabels,
	}

	id, err := startContainer(verbose, config, hostConfig, networkingConfig, containerName)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't start container %s\n%+v", containerName, err)
	}

	return id, nil
}

// deleting container
func removeContainer(ID string) error {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}
	//always force delete
	if err := docker.ContainerRemove(ctx, ID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("FAILURE: couldn't delete container [%s] -> %+v", ID, err)
	}
	return nil
}
