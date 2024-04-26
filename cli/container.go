package run

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerClient "github.com/docker/docker/client"
)

// To do: update this function and solve why the package function is not working
func IsImageNotFoundError(err error) bool {
	// Check if the error message contains a string indicating that the image is not found
	return strings.Contains(err.Error(), "No such image") || strings.Contains(err.Error(), "not found")
}

func startContainer(verbose bool, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, containerName string) (string, error) {
	ctx := context.Background()

	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	// first try createContainer by assuming the image is locally available
	// resp --> container create response. An object representing the response from Docker after creating the container. It contains information about the newly created container, such as its unique identifier (ID).
	resp, err := docker.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	// if any error from container start means no image found then pull the image
	if IsImageNotFoundError(err) {
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
		resp, err = docker.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
		if err != nil {
			return "", fmt.Errorf("ERROR: couldn't create container after pull %s\n%+v", containerName, err)
		}
	} else if err != nil { // if any other error other than image not found happens
		return "", fmt.Errorf("ERROR: couldn't create container %s\n%+v", containerName, err)
	}

	// start the container
	if err := docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func createServer(verbose bool, image string, port string, args []string, env []string, name string, volumes []string) (string, error) {
	log.Printf("Creating server using %s...\n", image)

	containerLabels := make(map[string]string)
	containerLabels["app"] = "k3d"
	containerLabels["component"] = "server"
	containerLabels["created"] = time.Now().Format("2006-01-02 15:04:05")
	containerLabels["cluster"] = name

	containerName := fmt.Sprintf("k3d-%s-server", name)

	containerPort := nat.Port(fmt.Sprintf("%s/tcp", port))

	//handle hostconfig
	hostConfig := &container.HostConfig{
		// Port mapping between the exposed port (container) and the host
		// Key = containerPort. Represents the port inside the container
		// Value = []nat.PortBinding. Represents the port on the host machine. Each nat.PortBinding struct specifies the mapping of a container port to a host port.
		PortBindings: nat.PortMap{
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
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			name: {
				Aliases: []string{containerName},
			},
		},
	}

	// Config contains the configuration data about a container. It should hold only portable information about the container. Here, "portable" means "independent from the host we are running on"
	config := &container.Config{
		Image: image,
		Cmd:   append([]string{"server"}, args...),
		ExposedPorts: nat.PortSet{
			containerPort: struct{}{},
		},
		Env:    env,
		Labels: containerLabels,
	}
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
		Image:  image,
		Env:    env,
		Labels: containerLabels,
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
