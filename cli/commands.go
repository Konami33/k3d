package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerClient "github.com/docker/docker/client"
	"github.com/urfave/cli"
)

const (
	defaultRegistry    = "docker.io"
	defaultServerCount = 1
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
	// Ping pings the server and returns the value of the "Docker-Experimental", "Builder-Version", "OS-Type" & "API-Version" headers.
	ping, err := docker.Ping(ctx)
	if err != nil {
		return fmt.Errorf("ERROR: checking docker failed\n%+v", err)
	}
	log.Printf("SUCCESS: Checking docker succeeded (API: v%s)\n", ping.APIVersion)
	return nil
}

// CreateCluster creates a new single-node cluster container and initializes the cluster directory
func CreateCluster(c *cli.Context) error {

	//handle cluster name
	if err := CheckClusterName(c.String("name")); err != nil {
		return err
	}	
	
	// Check for cluster existence before using a name to create a new cluster
	if cluster, err := getClusters(false, c.String("name")); err != nil {
		return err
	} else if len(cluster) != 0 {
		// A cluster exists with the same name. Return with an error.
		return fmt.Errorf("ERROR: Cluster %s already exists", c.String("name"))
	}

	// define image
	image := c.String("image") //for now: docker.io/rancher/k3s:latest
	if c.IsSet("version") {
		// TODO: --version to be deprecated
		log.Println("[WARNING] The `--version` flag will be deprecated soon, please use `--image rancher/k3s:<version>` instead")
		if c.IsSet("image") {
			// version specified, custom image = error (to push deprecation of version flag)
			log.Fatalln("[ERROR] Please use `--image <image>:<version>` instead of --image and --version")
		} else {
			// version specified, default image = ok (until deprecation of version flag)
			// docker.io/rancher/k3s:
			image = fmt.Sprintf("%s:%s", strings.Split(image, ":")[0], c.String("version"))
		}
	}
	if len(strings.Split(image, "/")) <= 2 {
		// fallback to default registry
		image = fmt.Sprintf("%s/%s", defaultRegistry, image)
	}

	// create cluster network
	networkID, err := createClusterNetwork(c.String("name"))
	if err != nil {
		return err
	}
	log.Printf("Created cluster network with ID %s", networkID)

	// environment variables
	env := []string{"K3S_KUBECONFIG_OUTPUT=/output/kubeconfig.yaml"}
	if c.IsSet("env") || c.IsSet("e") {
		env = append(env, c.StringSlice("env")...)
	}

	// clusterSecret and token is a must. otherwise we can't join the cluster
	k3sClusterSecret := ""
	k3sToken := ""

	//if worker node is set append the cluster secret and token to the environment variables
	if c.Int("workers") > 0 {
		k3sClusterSecret = fmt.Sprintf("K3S_CLUSTER_SECRET=%s", GenerateRandomString(20))
		env = append(env, k3sClusterSecret)

		k3sToken = fmt.Sprintf("K3S_TOKEN=%s", GenerateRandomString(20))
		env = append(env, k3sToken)
	}

	if c.IsSet("port") {
		// log.Println("WARNING: As of v2.0.0 --port will be used for arbitrary port-mappings. It's original functionality can then be used via --api-port.")
		log.Println("INFO: As of v2.0.0 --port will be used for arbitrary port mapping. Please use --api-port/-a instead for configuring the Api Port")
	}

	// constructs the arguments to be passed to the k3s server
	k3sServerArgs := []string{"--https-listen-port", c.String("api-port")}
	if c.IsSet("server-arg") || c.IsSet("x") {
		k3sServerArgs = append(k3sServerArgs, c.StringSlice("server-arg")...)
	}

	portmap, err := mapNodesToPortSpecs(c.StringSlice("publish"), GetAllContainerNames(c.String("name"), defaultServerCount, c.Int("workers")))
	if err != nil {
		log.Fatal(err)
	}

	// let's go
	log.Printf("Creating cluster [%s]", c.String("name"))

	// create a k3s server container by passing the arguments
	// createServer creates a new server container
	// dockerID is the ID of the container
	// container.go -> createServer()
	dockerID, err := createServer(
		c.GlobalBool("verbose"),
		image,
		c.String("api-port"),
		k3sServerArgs,
		env,
		c.String("name"),
		c.StringSlice("volume"),
		portmap,
	)
	if err != nil {
		log.Printf("ERROR: failed to create cluster\n%+v", err)
		// Delete cluster if it is not started due to port confliction or any other unseen reason
		delErr := DeleteCluster(c)
		if delErr != nil {
			return delErr
		}
		os.Exit(1)
	}
	ctx := context.Background()
	// dockerClient provides a client library for interacting with the Docker Engine API
	// FromEnv is a function that returns a client.Client that is configured from the environment.
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}

	// check if --timeout flag is set. Log a message to show the deprecation
	if c.IsSet("timeout") {
		log.Println("[Warning] The --timeout flag is deprecated. use '--wait <timeout>' instead")
	}

	// wait for k3s to be up and running if we want it
	start := time.Now()
	timeout := time.Duration(c.Int("wait")) * time.Second //timeout time calc

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
			ShowStdout: true,
			ShowStderr: true,
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

	// creating the specified worker nodes
	if c.Int("workers") > 0 {
		k3sWorkerArgs := []string{}
		// appending the k3sClusterSecret and k3sToke to env variable
		env := []string{k3sClusterSecret, k3sToken}
		log.Printf("Booting %s workers for cluster %s", strconv.Itoa(c.Int("workers")), c.String("name"))
		for i := 0; i < c.Int("workers"); i++ {
			workerID, err := createWorker(
				c.GlobalBool("verbose"),
				image,
				k3sWorkerArgs,
				env,
				c.String("name"),
				c.StringSlice("volume"),
				i, //postfix
				c.String("api-port"),
				portmap, // All ports exposed by --publish will also be exported for all worker
				c.Int("port-auto-offset"),
			)
			if err != nil {
				// if worker creation fails, delete the cluster and exit. Atomic creation
				log.Printf("ERROR: failed to create worker node for cluster %s\n%+v", c.String("name"), err)
				delErr := DeleteCluster(c)
				if delErr != nil {
					return delErr
				}
				os.Exit(1)
			}
			log.Printf("Created worker with ID %s\n", workerID)
		}
	}
	// after server and worker node creation showing this message
	log.Printf("SUCCESS: created cluster [%s]", c.String("name"))
	log.Printf(`You can now use the cluster with:

export KUBECONFIG="$(%s get-kubeconfig --name='%s')"
kubectl cluster-info`, os.Args[0], c.String("name"))

	return nil
}

// DeleteCluster removes the cluster container and its cluster directory
func DeleteCluster(c *cli.Context) error {
	
	clusters, err := getClusters(c.Bool("all"), c.String("name"))
	if err != nil {
		return err
	}

	// remove cluster one by one
	for _, cluster := range clusters {
		log.Printf("Removing cluster [%s]", cluster.name)
		// first delete workder node
		if len(cluster.workers) > 0 {
			log.Printf("...Removing %d workers\n", len(cluster.workers))
			// iterate over all the worker node and delete each one
			for _, worker := range cluster.workers {
				//removeContainer defined in container.go used to deleteContianer
				if err := removeContainer(worker.ID); err != nil {
					log.Println(err)
					continue
				}
			}
		}
		//now remove the k3d server
		log.Println("...Removing server")
		//directory
		deleteClusterDir(cluster.name)
		if err := removeContainer(cluster.server.ID); err != nil {
			return fmt.Errorf("ERROR: Couldn't remove server for cluster %s\n%+v", cluster.name, err)
		}

		// deleting the cluster network
		log.Println("...Removing cluster network")
		if err := deleteClusterNetwork(cluster.name); err != nil {
			log.Printf("WARNING: couldn't delete cluster network for cluster %s\n%+v", cluster.name, err)
		}

		log.Printf("SUCCESS: removed cluster [%s]", cluster.name)
	}
	return nil
}

// StopCluster stops a running cluster container (restartable)
func StopCluster(c *cli.Context) error {
	
	clusters, err := getClusters(c.Bool("all"), c.String("name"))
	if err != nil {
		return err
	}

	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	// remove clusters one by one instead of appending all names to the docker command
	// this allows for more granular error handling and logging
	for _, cluster := range clusters {
		log.Printf("Stopping cluster [%s]", cluster.name)
		// handle workers
		if len(cluster.workers) > 0 {
			log.Printf("...Stopping %d workers\n", len(cluster.workers))
			for _, worker := range cluster.workers {
				if err := docker.ContainerStop(ctx, worker.ID, container.StopOptions{}); err != nil {
					log.Println(err)
					continue
				}
			}
		}
		log.Println("...Stopping server")
		//now stop the server
		if err := docker.ContainerStop(ctx, cluster.server.ID, container.StopOptions{}); err != nil {
			return fmt.Errorf("ERROR: Couldn't stop server for cluster %s\n%+v", cluster.name, err)
		}

		log.Printf("SUCCESS: Stopped cluster [%s]", cluster.name)
	}

	return nil
}

// StartCluster starts a stopped cluster container
func StartCluster(c *cli.Context) error {
	clusters, err := getClusters(c.Bool("all"), c.String("name"))

	if err != nil {
		return err
	}

	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	for _, cluster := range clusters {
		log.Printf("Starting cluster [%s]", cluster.name)

		log.Println("...Starting server")
		// first start the server container
		if err := docker.ContainerStart(ctx, cluster.server.ID, container.StartOptions{}); err != nil {
			return fmt.Errorf("ERROR: Couldn't start server for cluster %s\n%+v", cluster.name, err)
		}

		//if any worker node start them
		if len(cluster.workers) > 0 {
			log.Printf("...Starting %d workers\n", len(cluster.workers))
			for _, worker := range cluster.workers {
				if err := docker.ContainerStart(ctx, worker.ID, container.StartOptions{}); err != nil {
					log.Println(err)
					continue
				}
			}
		}
		log.Printf("SUCCESS: Started cluster [%s]", cluster.name)
	}
	return nil
}

// ListClusters prints a list of created clusters
func ListClusters(c *cli.Context) error {
	if c.IsSet("all") {
		log.Println("INFO: --all is on by default, thus no longer required. This option will be removed in v2.0.0")
	}
	printClusters()
	return nil
}

// GetKubeConfig grabs the kubeconfig from the running cluster and prints the path to stdout
func GetKubeConfig(c *cli.Context) error {
	// sourcePath := fmt.Sprintf("k3d-%s-server:/output/kubeconfig.yaml", c.String("name"))
	// destPath, _ := getClusterDir(c.String("name"))
	// cmd := "docker"
	// args := []string{"cp", sourcePath, destPath}

	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return err
	}

	filters := filters.NewArgs()
	filters.Add("label", "app=k3d")
	filters.Add("label", fmt.Sprintf("cluster=%s", c.String("name")))
	filters.Add("label", "component=server")

	//ContainerList returns the list of containers/servers in the docker host.
	server, err := docker.ContainerList(ctx, container.ListOptions{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("failed to get server container for cluster %s\n%+v", c.String("name"), err)
	}
	if len(server) == 0 {
		return fmt.Errorf("no server container for cluster %s", c.String("name"))
	}

	// get kubeconfig file from container and read contents
	// CopyFromContainer gets the content from the container and returns it as a Reader for a TAR archive to manipulate it in the host.
	// sourcePath := fmt.Sprintf("k3d-%s-server:/output/kubeconfig.yaml", c.String("name"))
	// destPath, _ := getClusterDir(c.String("name"))
	reader, _, err := docker.CopyFromContainer(ctx, server[0].ID, "/output/kubeconfig.yaml")
	if err != nil {
		return fmt.Errorf("ERROR: couldn't copy kubeconfig.yaml from server container %s\n%+v", server[0].ID, err)
	}
	// It's up to the caller to close the reader.
	defer reader.Close()

	// ReadAll reads from reader until an error or EOF and returns the data it read.
	readBytes, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't read kubeconfig from container\n%+v", err)
	}

	// create destination kubeconfig file
	// getClusterDir returns the path to the cluster directory: $HOME/.config/k3d/<cluster_name>
	clusterDir, err := getClusterDir(c.String("name"))
	destPath := fmt.Sprintf("%s/kubeconfig.yaml", clusterDir)
	if err != nil {
		return err
	}

	//Create creates or truncates the named file. If the file already exists, it is truncated. If the file does not exist, it is created with mode 0666 (before umask). If successful, methods on the returned File can be used for I/O;
	kubeconfigfile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't create kubeconfig.yaml in %s\n%+v", clusterDir, err)
	}
	//Close closes the File, rendering it unusable for I/O.
	// defer: Execute this line just before leaving the function."
	defer kubeconfigfile.Close()

	// write to file, skipping the first 512 bytes which contain file metadata and trimming any NULL characters
	//Write writes len(b) bytes from b to the File. It returns the number of bytes written and an error, if any. Write returns a non-nil error when n != len(b).
	//bytes.Trim(..., "\x00"): This function call trims any trailing NULL (\x00) characters from the sliced byte slice. It ensures that only valid data is written to the file.
	_, err = kubeconfigfile.Write(bytes.Trim(readBytes[512:], "\x00"))
	if err != nil {
		return fmt.Errorf("ERROR: couldn't write to kubeconfig.yaml\n%+v", err)
	}

	// output kubeconfig file path to stdout
	fmt.Println(destPath)

	return nil
}
