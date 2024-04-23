package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"github.com/urfave/cli"
)

// CheckTools checks if the installed tools work correctly
// command: docker version
func CheckTools(c *cli.Context) error {
	log.Print("Checking docker...")
	ctx := context.Background()

	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}
	ping, err := docker.Ping(ctx)
	if err != nil {
		return fmt.Errorf("ERROR: checking docker failed\n%+v", err)
	}
	log.Printf("SUCCESS: Checking docker succeeded (API: v%s)\n", ping.APIVersion)
	return nil
}

// CreateCluster creates a new single-node cluster container and initializes the cluster directory
func CreateCluster(c *cli.Context) error {
	// Check if the timeout flag is set but the wait flag is not, return an error if so
	if c.IsSet("timeout") && !c.IsSet("wait") {
		return errors.New("can not use --timeout flag without --wait flag")
	}
	// constructs the arguments to be passed to the k3s server
	k3sServerArgs := []string{"--https-listen-port", c.String("port")}
	if c.IsSet("server-arg") || c.IsSet("x") {
		k3sServerArgs = append(k3sServerArgs, c.StringSlice("server-arg")...)
	}
	// let's go
	log.Printf("Creating cluster [%s]", c.String("name"))

	// create a k3s server container by passing the arguments
	// createServer creates a new server container
	// dockerID is the ID of the container
	// container.go -> createServer()
	dockerID, err := createServer(
		c.Bool("verbose"),
		fmt.Sprintf("docker.io/rancher/k3s:%s", c.String("version")),
		c.String("port"),
		k3sServerArgs,
		[]string{"K3S_KUBECONFIG_OUTPUT=/output/kubeconfig.yaml"},
		c.String("name"),
		strings.Split(c.String("volume"), ","), //value: "dir1:mount1,dir2:mount2" --> []string{"dir1:mount1", "dir2:mount2"}
	)
	if err != nil {
		log.Fatalf("ERROR: failed to create cluster\n%+v", err)
	}
	ctx := context.Background()
	// dockerClient provides a client library for interacting with the Docker Engine API
	// FromEnv is a function that returns a client.Client that is configured from the environment.
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}
	// wait for k3s to be up and running if we want it
	start := time.Now()
	timeout := time.Duration(c.Int("timeout")) * time.Second //timeout time calc

	// infinite loop until wait is false
	for c.IsSet("wait") {
		// if timeout is set and time is up, delete the cluster and return an error
		if timeout != 0 && !time.Now().After(start.Add(timeout)) {
			err := DeleteCluster(c)
			if err != nil {
				return err
			}
			return errors.New("cluster creation exceeded specified timeout")
		}
		// get the docker logs of the created container
		// ContainerLogs returns the logs generated by a container in an io.ReadCloser. It's up to the caller to close the stream.
		// The options parameter allows to specify the options of the logs.
		out, err := docker.ContainerLogs(ctx, dockerID, container.LogsOptions{
			ShowStdout: true, ShowStderr: true,
		})
		if err != nil {
			out.Close() //closes the buffer
			return fmt.Errorf("ERROR: couldn't get docker logs for %s\n%+v", c.String("name"), err)
		}
		// represents a buffer for bytes data.
		// The new keyword used to allocate memory for a new value of a specified type. It
		// allocates memory for a new bytes.Buffer value and initializes it with its zero value.
		//The buf variable is declared to hold a pointer to a bytes.Buffer object.
		buf := new(bytes.Buffer)
		// ReadFrom reads data from r until EOF or error. The return value n is the number of bytes read. The data is read into buf.
		nRead, _ := buf.ReadFrom(out)
		// Close closes the buffer.
		out.Close()
		// output is the string representation of the buffer
		output := buf.String()
		// the loop continuously checks the Docker logs of the created container for the message "Running kubelet"
		// if the message is found, the loop is broken
		if nRead > 0 && strings.Contains(string(output), "Running kubelet") {
			break
		}
		//delay for one second and try again
		time.Sleep(1 * time.Second)
	}
	// creating a cluster directory
	createClusterDir(c.String("name"))
	log.Printf("SUCCESS: created cluster [%s]", c.String("name"))
	log.Printf(`You can now use the cluster with:

export KUBECONFIG="$(%s get-kubeconfig --name='%s')"
kubectl cluster-info`, os.Args[0], c.String("name"))
	return nil
}

// DeleteCluster removes the cluster container and its cluster directory
func DeleteCluster(c *cli.Context) error {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}

	clusterNames := []string{}
	// if `--all` is specified, get all the cluster names, otherwise only the specified one
	if !c.Bool("all") {
		clusterNames = append(clusterNames, c.String("name"))
	} else {
		clusterList, err := getClusterNames()
		if err != nil {
			return fmt.Errorf("ERROR: `--all` specified, but no clusters were found\n%v", err)
		}
		clusterNames = append(clusterNames, clusterList...)
	}
	// delete each cluster by iterating over the list of cluster names
	for _, name := range clusterNames {
		log.Printf("Deleting cluster [%s]", name)
		// get the cluster info
		cluster, err := getCluster(name)
		if err != nil {
			log.Printf("WARNING: couldn't get docker info for %s", name)
			continue
		}
		log.Printf("WARNING: couldn't delete cluster [%s], trying a force remove now.", cluster.name)
		// ContainerRemove kills and removes a container from the docker host.
		if err := docker.ContainerRemove(ctx, cluster.id, container.RemoveOptions{Force: true});
		err != nil {
			log.Printf("FAILURE: couldn't delete cluster container for [%s] -> %+v", cluster.name, err)
		}
		// deleting the cluster directory
		deleteClusterDir(cluster.name)
		log.Printf("SUCCESS: removed cluster [%s]", cluster.name)
	}
	return nil
}

// StopCluster stops a running cluster container (restartable)
func StopCluster(c *cli.Context) error {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}
	
	clusterNames := []string{}

	//handle the -all flag
	if !c.Bool("all") {
		clusterNames = append(clusterNames, c.String("name"))
	} else {
		clusterList, err := getClusterNames()
		if err != nil {
			return fmt.Errorf("ERROR: `--all` specified, but no clusters were found\n%v", err)
		}
		clusterNames = append(clusterNames, clusterList...)
	}

	// iterate over the cluster and stop one by one
	for _, name := range clusterNames {
		log.Printf("Stopping cluster [%s]", name)
		cluster, err := getCluster(name)
		if err != nil {
			log.Printf("WARNING: couldn't get docker info for %s", name)
			continue
		}
		if err := docker.ContainerStop(ctx, cluster.id, container.StopOptions{}); err != nil {
			fmt.Printf("WARNING: couldn't stop cluster %s\n%+v", cluster.name, err)
			continue
		}
		log.Printf("SUCCESS: stopped cluster [%s]", cluster.name)
	}
	return nil
}

// StartCluster starts a stopped cluster container
func StartCluster(c *cli.Context) error {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}

	clusterNames := []string{}

	if !c.Bool("all") {
		clusterNames = append(clusterNames, c.String("name"))
	} else {
		clusterList, err := getClusterNames()
		if err != nil {
			return fmt.Errorf("ERROR: `--all` specified, but no clusters were found\n%v", err)
		}
		clusterNames = append(clusterNames, clusterList...)
	}

	for _, name := range clusterNames {
		log.Printf("Stopping cluster [%s]", name)
		cluster, err := getCluster(name)
		if err != nil {
			log.Printf("WARNING: couldn't get docker info for %s", name)
			continue
		}
		if err := docker.ContainerStart(ctx, cluster.id, container.StartOptions{}); err != nil {
			fmt.Printf("WARNING: couldn't start cluster %s\n%+v", cluster.name, err)
			continue
		}
		log.Printf("SUCCESS: started cluster [%s]", cluster.name)
	}
	return nil
}

// ListClusters prints a list of created clusters
func ListClusters(c *cli.Context) error {
	printClusters(c.Bool("all"))
	return nil
}

// GetKubeConfig grabs the kubeconfig from the running cluster and prints the path to stdout
func GetKubeConfig(c *cli.Context) error {
	//getting the source path and dest path or directory
	sourcePath := fmt.Sprintf("%s:/output/kubeconfig.yaml", c.String("name"))
	destPath, _ := getClusterDir(c.String("name"))
	cmd := "docker"
	args := []string{"cp", sourcePath, destPath}
	// GlobalBool looks up the value of a global BoolFlag, returns false if not found
	if err := runCommand(c.GlobalBool("verbose"), cmd, args...); err != nil {
		return fmt.Errorf("ERROR: Couldn't get kubeconfig for cluster [%s]\n%+v", fmt.Sprintf("%s-server", c.String("name")), err)
	}
	fmt.Printf("%s\n", path.Join(destPath, "kubeconfig.yaml"))
	return nil
}
