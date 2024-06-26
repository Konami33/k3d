package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
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

	// On Error delete the cluster.  If there createCluster() encounter any error,
	// call this function to remove all resources allocated for the cluster so far
	// so that they don't linger around.
	deleteCluster := func() {
		if err := DeleteCluster(c); err != nil {
			log.Printf("Error: Failed to delete cluster %s", c.String("name"))
		}
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
	env = append(env, c.StringSlice("env")...)

	// clusterSecret and token is a must. otherwise we can't join the server with workers
	k3sClusterSecret := ""
	k3sToken := ""

	//The cluster secret and token to the environment variables
	k3sClusterSecret = fmt.Sprintf("K3S_CLUSTER_SECRET=%s", GenerateRandomString(20))
	k3sToken = fmt.Sprintf("K3S_TOKEN=%s", GenerateRandomString(20))
	env = append(env, k3sClusterSecret, k3sToken)

	if c.IsSet("port") {
		// log.Println("WARNING: As of v2.0.0 --port will be used for arbitrary port-mappings. It's original functionality can then be used via --api-port.")
		log.Println("INFO: As of v2.0.0 --port will be used for arbitrary port mapping. Please use --api-port/-a instead for configuring the Api Port")
	}

	apiPort, err := parseAPIPort(c.String("api-port"))
	if err != nil {
		return err
	}

	k3sServerArgs := []string{"--https-listen-port", apiPort.Port}

	// see why docker client doesn't pay attention to DOCKER_MACHINE_NAME..
	// 	It turns out that docker client only pays attention to the following
	// 	environment variables:
	// 	DOCKER_HOST to set the url to the docker server.
	// 	DOCKER_API_VERSION to set the version of the API to reach, leave empty for latest.
	// 	DOCKER_CERT_PATH to load the TLS certificates from.
	// 	DOCKER_TLS_VERIFY to enable or disable TLS verification, off by default.
	// 	A miss configured DOCKER_MACHINE_NAME won't affect docker client, so k3d
	// 	should just ignore the error.

	if apiPort.Host == "" {
		apiPort.Host, err = getDockerMachineIp()
		// IP address is the same as the host
		apiPort.HostIP = apiPort.Host
		if err != nil {
			log.Printf("WARNING: Failed to get docker machine IP address, ignoring the DOCKER_MACHINE_NAME environment variable setting.\n")
		}
	}

	if apiPort.Host != "" {
		// Add TLS SAN for non default host name
		log.Printf("Add TLS SAN for %s", apiPort.Host)
		k3sServerArgs = append(k3sServerArgs, "--tls-san", apiPort.Host)
	}

	if c.IsSet("server-arg") || c.IsSet("x") {
		k3sServerArgs = append(k3sServerArgs, c.StringSlice("server-arg")...)
	}

	portmap, err := mapNodesToPortSpecs(c.StringSlice("publish"), GetAllContainerNames(c.String("name"), defaultServerCount, c.Int("workers")))
	if err != nil {
		log.Fatal(err)
	}

	clusterSpec := &ClusterSpec{
		AgentArgs:         []string{},
		APIPort:           *apiPort,
		AutoRestart:       c.Bool("auto-restart"),
		ClusterName:       c.String("name"),
		Env:               env,
		Image:             image,
		NodeToPortSpecMap: portmap,
		PortAutoOffset:    c.Int("port-auto-offset"),
		ServerArgs:        k3sServerArgs,
		Verbose:           c.GlobalBool("verbose"),
		Volumes:           c.StringSlice("volume"),
	}

	// let's go
	log.Printf("Creating cluster [%s]", c.String("name"))

	// create a k3s server container by passing the arguments
	// createServer creates a new server container
	// dockerID is the ID of the container
	// container.go -> createServer()

	// create the directory where we will put the kubeconfig file by default (when running `k3d get-config`)
	createClusterDir(c.String("name"))
	dockerID, err := createServer(clusterSpec)
	if err != nil {
		deleteCluster()
		return err
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
	timeout := time.Duration(c.Int("wait")) * time.Second //timeout time calc

	// infinite loop until wait is false
	for c.IsSet("wait") {
		// if timeout is set and time is up, delete the cluster and return an error
		if timeout != 0 && !time.Now().After(start.Add(timeout)) {
			deleteCluster() //literal function
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

	// creating the specified worker nodes
	if c.Int("workers") > 0 {
		// k3sWorkerArgs := []string{}
		// // appending the k3sClusterSecret and k3sToke to env variable
		// env := []string{k3sClusterSecret, k3sToken}
		// // passing the environment variables to the workers
		// env = append(env, c.StringSlice("env")...)
		log.Printf("Booting %s workers for cluster %s", strconv.Itoa(c.Int("workers")), c.String("name"))
		for i := 0; i < c.Int("workers"); i++ {
			workerID, err := createWorker(clusterSpec, i)
			if err != nil {
				// if worker creation fails, delete the cluster and exit. Atomic creation
				deleteCluster() // literal function
				return err
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

	cluster := c.String("name")
	// create destination kubeconfig file
	// destPath = getClusterDir/kubeconfig.yaml
	// clusterDir = $HOME/.config/k3d/<cluster_name>
	kubeConfigPath, err := getKubeConfig(cluster)
	if err != nil {
		return err
	}
	// output kubeconfig file path to stdout
	fmt.Println(kubeConfigPath)
	return nil
}

// Bash function
func Shell(c *cli.Context) error {
	return subShell(c.String("name"), c.String("shell"), c.String("command"))
}

// ImportImage saves an image locally and imports it into the k3d containers
func ImportImage(c *cli.Context) error {
	return importImage(c.String("name"), c.String("image"))
}
