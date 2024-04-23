package run

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerClient "github.com/docker/docker/client"
	"github.com/mitchellh/go-homedir"
	"github.com/olekukonko/tablewriter"
)

type cluster struct {
	name   string
	image  string
	status string
	ports  []string //slice of string. Can holde multiple string value
	id     string
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
	clusterNames, err := getClusterNames()
	if err != nil {
		log.Fatalf("ERROR: Couldn't list clusters -> %+v", err)
	}
	if len(clusterNames) == 0 {
		log.Printf("No clusters found!")
		return
	}

	//creating a table output with header name, image, status
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"NAME", "IMAGE", "STATUS"})

	for _, clusterName := range clusterNames {
		cluster, _ := getCluster(clusterName)
		clusterData := []string{cluster.name, cluster.image, cluster.status}
		if cluster.status == "running" || all {
			table.Append(clusterData)
		}
	}
	table.Render()
}

// getClusterNames returns a list of cluster names which are folder names in the config directory
func getClusterNames() ([]string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		log.Printf("ERROR: Couldn't get user's home directory")
		return nil, err
	}
	configDir := path.Join(homeDir, ".config", "k3d")
	files, err := os.ReadDir(configDir)
	if err != nil {
		log.Printf("ERROR: Couldn't list files in [%s]", configDir)
		return nil, err
	}
	clusters := []string{}
	for _, file := range files {
		if file.IsDir() {
			clusters = append(clusters, file.Name())
		}
	}
	return clusters, nil
}

// returns information about a specific cluster
// takes cluster name as input and returns cluster struct containing details(name, image, status)
// if any error occcured, returns error
func getCluster(name string) (cluster, error) {
	//initalize cluster with default value
	cluster := cluster{
		name:   name,
		image:  "UNKNOWN",
		status: "UNKNOWN",
		ports:  []string{"UNKNOWN"},
		id:     "UNKNOWN",
	}

	ctx := context.Background()

	//docker client
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		log.Printf("WARNING: couldn't create docker client -> %+v", err)
		return cluster, err
	}

	filters := filters.NewArgs()
	filters.Add("label", "app=k3d")
	filters.Add("label", fmt.Sprintf("cluster=%s", cluster.name))
	filters.Add("label", "component=server")

	// container.Listoptions will work
	// ContainerList returns the list of containers in the docker host.
	containerList, err := docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return cluster, fmt.Errorf("WARNING: couldn't get docker info for [%s] -> %+v", cluster.name, err)
	}
	container := containerList[0]
	cluster.image = container.Image
	cluster.status = container.State
	for _, port := range container.Ports {
		cluster.ports = append(cluster.ports, strconv.Itoa(int(port.PublicPort)))
	}
	cluster.id = container.ID
	return cluster, nil
}
