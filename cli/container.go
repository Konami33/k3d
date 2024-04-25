package run

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	dockerClient "github.com/docker/docker/client"
)

func createServer(verbose bool, image string, port string, args []string, env []string, name string, volumes []string) (string, error) {
	log.Printf("Creating server using %s...\n", image)
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}
	reader, err := docker.ImagePull(ctx, image, types.ImagePullOptions{})
	//reader, err := docker.ImagePull(ctx, image, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't pull image %s\n%+v", image, err)
	}
	//problem
	if verbose {
		_, err := io.Copy(os.Stdout, reader) // TODO: only if verbose mode
		if err != nil {
			log.Printf("WARNING: couldn't get docker output\n%+v", err)
		}
	} else {
		_, err := io.Copy(io.Discard, reader)
		if err != nil {
			log.Printf("WARNING: couldn't get docker output\n%+v", err)
		}
	}
	// container basic information
	containerLabels := make(map[string]string) //initializes an empty map string --> string
	containerLabels["app"] = "k3d"
	containerLabels["component"] = "server"
	containerLabels["created"] = time.Now().Format("2006-01-02 15:04:05")
	containerLabels["cluster"] = name

	containerName := fmt.Sprintf("k3d-%s-server", name)

	// It is used to define port bindings and exposed ports for Docker containers. represents or holding a network port
	// if port is 8080: it represens port 8080 with the TCP protocol.
	containerPort := nat.Port(fmt.Sprintf("%s/tcp", port))

	//host config
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			// Key = containerPort. Represents the port inside the container
			// Value = []nat.PortBinding. Represents the port on the host machine. Each nat.PortBinding struct specifies the mapping of a container port to a host port.
			containerPort: []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: port,
				},
			},
		},
		Privileged: true,
	}
	//handle volume
	if len(volumes) > 0 && volumes[0] != "" {
		hostConfig.Binds = volumes
	}

	//networkingConfig
	// this specifies how the container interacts with Docker networks
	networkingConfig := &network.NetworkingConfig{
		// EndpointSettings stores the network endpoint details
		// This is a map holds the network configuration for different endpoints.
		// values are pointers to network.EndpointSettings structs.
		EndpointsConfig: map[string]*network.EndpointSettings{
			// This is the key of the map, which represents the name of the network endpoint.
			// Aliases: []string{containerName}: value associated with the name key. It's a pointer to a network.EndpointSettings struct. The Aliases field of this struct is set to an array containing a single string, which is containerName. This means that the endpoint identified by name will have an alias, and that alias is containerName.
			name: {
				Aliases: []string{containerName},
			},
		},
	}

	// problem
	resp, err := docker.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   append([]string{"server"}, args...),
		ExposedPorts: nat.PortSet{ // nat.PortSet{} -> used to manage a collection of network ports.
			containerPort: struct{}{}, //struct{}{} -> empty struct. Doesn't contain any fields or memory. used when we only need to signal the presence of something without carrying any additional data.
		},
		Env:    env,
		Labels: containerLabels,
	}, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create container %s\n%+v", containerName, err)
	}
	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("ERROR: couldn't start container %s\n%+v", containerName, err)
	}
	// resp: This variable contains the response from the Docker API after attempting to create a container. It typically includes information about the created container, such as its ID, name, and other metadata.
	return resp.ID, nil
}

// creating worker node
func createWorker(verbose bool, image string, args []string, env []string, name string, volumes []string, postfix string, serverPort string) (string, error) {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	//pull the k3s image
	reader, err := docker.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't pull image %s\n%+v", image, err)
	}
	//prints the docker output to the console if verbose flag is set
	if verbose {
		_, err := io.Copy(os.Stdout, reader)
		if err != nil {
			log.Printf("WARNING: couldn't get docker output\n%+v", err)
		}
	}
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

	resp, err := docker.ContainerCreate(ctx, &container.Config{
		Image:  image,
		Env:    env,
		Labels: containerLabels,
	}, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create container %s\n%+v", containerName, err)
	}

	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("ERROR: couldn't start container %s\n%+v", containerName, err)
	}

	return resp.ID, nil
}

// deleting container
func removeContainer(ID string) error {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}
	if err := docker.ContainerRemove(ctx, ID, container.RemoveOptions{}); err != nil {
		log.Printf("WARNING: couldn't delete container [%s], trying a force remove now.", ID)
		if err := docker.ContainerRemove(ctx, ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("FAILURE: couldn't delete container [%s] -> %+v", ID, err)
		}
	}
	return nil
}
