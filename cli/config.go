package run

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerClient "github.com/docker/docker/client"
	"github.com/mitchellh/go-homedir"
	"github.com/olekukonko/tablewriter"
)

type cluster struct {
	name        string
	image       string
	status      string
	serverPorts []string
	// types.Container is a struct type defined in the Docker API package.
	// It represents information about a Docker container, such as its ID, name, image, state, and other attributes.
	server      types.Container
	workers     []types.Container
}

// createDirIfNotExists checks for the existence of a directory and creates it along with all required parents if not.
// It returns an error if the directory (or parents) couldn't be created and nil if it worked fine or if the path already exists.
func createDirIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}
	return nil
}

// createClusterDir creates a directory with the cluster name under $HOME/.config/k3d/<cluster_name>.
// The cluster directory will be used e.g. to store the kubeconfig file.
func createClusterDir(name string) {
	clusterPath, _ := getClusterDir(name)
	if err := createDirIfNotExists(clusterPath); err != nil {
		log.Fatalf("ERROR: couldn't create cluster directory [%s] -> %+v", clusterPath, err)
	}
}

// deleteClusterDir contrary to createClusterDir, this deletes the cluster directory under $HOME/.config/k3d/<cluster_name>
func deleteClusterDir(name string) {
	clusterPath, _ := getClusterDir(name)
	if err := os.RemoveAll(clusterPath); err != nil {
		log.Printf("WARNING: couldn't delete cluster directory [%s]. You might want to delete it manually.", clusterPath)
	}
}

// getClusterDir returns the path to the cluster directory which is $HOME/.config/k3d/<cluster_name>
func getClusterDir(name string) (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		log.Printf("ERROR: Couldn't get user's home directory")
		return "", err
	}
	return path.Join(homeDir, ".config", "k3d", name), nil
}

// printClusters prints the names of existing clusters
func printClusters(all bool) {
	clusters, err := getClusters()
	if err != nil {
		log.Fatalf("ERROR: Couldn't list clusters\n%+v", err)
	}
	if len(clusters) == 0 {
		log.Printf("No clusters found!")
		return
	}

	//creating a table output with header name, image, status
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"NAME", "IMAGE", "STATUS", "WORKERS"})

	for _, cluster := range clusters {
		workersRunning := 0
		for _, worker := range cluster.workers {
			if worker.State == "running" {
				workersRunning++
			}
		}
		workerData := fmt.Sprintf("%d/%d", workersRunning, len(cluster.workers))
		clusterData := []string{cluster.name, cluster.image, cluster.status, workerData}

		if cluster.status == "running" || all {
			table.Append(clusterData)
		}
	}
	table.Render()
}

// getClusterNames returns a list of cluster names which are folder names in the config directory
// func getClusterNames() ([]string, error) {
// 	homeDir, err := homedir.Dir()
// 	if err != nil {
// 		log.Printf("ERROR: Couldn't get user's home directory")
// 		return nil, err
// 	}
// 	configDir := path.Join(homeDir, ".config", "k3d")
// 	files, err := os.ReadDir(configDir)
// 	if err != nil {
// 		log.Printf("ERROR: Couldn't list files in [%s]", configDir)
// 		return nil, err
// 	}
// 	clusters := []string{}
// 	for _, file := range files {
// 		if file.IsDir() {
// 			clusters = append(clusters, file.Name())
// 		}
// 	}
// 	return clusters, nil
// }

// returns information about a specific cluster
// takes cluster name as input and returns cluster struct containing details(name, image, status)
// if any error occcured, returns error
func getClusters() (map[string]cluster, error) {

	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	filters := filters.NewArgs()
	filters.Add("label", "app=k3d")
	filters.Add("label", "component=server")

	//finding out the list of k3d-servers
	k3dServers, err := docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return nil, fmt.Errorf("WARNING: couldn't list server containers\n%+v", err)
	}

	clusters := make(map[string]cluster)
	// for worker node deleting the label "server" and adding "worker"
	filters.Del("label", "component=server")
	filters.Add("label", "component=worker")

	for _, server := range k3dServers {
		filters.Add("label", fmt.Sprintf("cluster=%s", server.Labels["cluster"]))
		//getting the worker nodes of each k3d server
		workers, err := docker.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters,
		})
		if err != nil {
			// return nil, fmt.Errorf("WARNING: couldn't list worker containers for cluster %s\n%+v", server.Labels["cluster"], err)
			log.Printf("WARNING: couldn't get worker containers for cluster %s\n%+v", server.Labels["cluster"], err)
		}
		serverPorts := []string{}
		for _, port := range server.Ports {
			serverPorts = append(serverPorts, strconv.Itoa(int(port.PublicPort)))
		}
		clusters[server.Labels["cluster"]] = cluster{
			name:        server.Labels["cluster"],
			image:       server.Image,
			status:      server.State,
			serverPorts: serverPorts,
			server:      server,
			workers:     workers,
		}
		// clear label filters before searching for next cluster
		filters.Del("label", fmt.Sprintf("cluster=%s", server.Labels["cluster"]))
	}
	return clusters, nil
}
// getCluster creates a cluster struct with populated information fields
func getCluster(name string) (cluster, error) {
	// get all clusters
	clusters, err := getClusters()
	//return the cluster with specified name
	return clusters[name], err
}
