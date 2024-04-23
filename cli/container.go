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
	dockerClient "github.com/docker/docker/client"
)

func createServer(verbose bool, image string, port string, args []string, env []string, name string, volumes []string) (string, error) {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	reader, err := docker.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't pull image %s\n%+v", image, err)
	}
	if verbose {
		_, err := io.Copy(os.Stdout, reader) // TODO: only if verbose mode
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

	containerName := fmt.Sprintf("%s-server", name)

	// It is used to define port bindings and exposed ports for Docker containers. represents or holding a network port
	// if port is 8080: it represens port 8080 with the TCP protocol.
	containerPort := nat.Port(fmt.Sprintf("%s/tcp", port))

	// problem
	resp, err := docker.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   append([]string{"server"}, args...),
		ExposedPorts: nat.PortSet{ // nat.PortSet{} -> used to manage a collection of network ports.
			containerPort: struct{}{}, //struct{}{} -> empty struct. Doesn't contain any fields or memory. used when we only need to signal the presence of something without carrying any additional data.
		},
		Env:    env,
		Labels: containerLabels,
	}, &container.HostConfig{ // creation of a HostConfig struct pointer to be used to configure the container's networking. Configuration settings for the host
		Binds: volumes,
		// This is a map that specifies how ports inside the container are mapped to ports on the host machine.
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
	}, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create container %s\n%+v", containerName, err)
	}
	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("ERROR: couldn't start container %s\n%+v", containerName, err)
	}
	// resp: This variable contains the response from the Docker API after attempting to create a container. It typically includes information about the created container, such as its ID, name, and other metadata.
	return resp.ID, nil

}
