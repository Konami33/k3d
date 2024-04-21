package main

import (
	"context"
	"log"
	"os"
	"path"

	dockerClient "github.com/docker/docker/client"
	"github.com/mitchellh/go-homedir"
	"github.com/olekukonko/tablewriter"
)

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
	//getting the home directory
	homeDir, err := homedir.Dir()
	if err != nil {
		log.Printf("ERROR: Couldn't get user's home directory")
		return "", err
	}
	// $HOME/.config/k3d/<cluster_name>
	return path.Join(homeDir, ".config", "k3d", name), nil
}

// printClusters prints the names of existing clusters
func printClusters(all bool) {
	clusters, err := getClusters()
	if err != nil {
		log.Fatalf("ERROR: Couldn't list clusters -> %+v", err)
	}
	// Get the docker client. Used to interact with docker to get information about containers
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		log.Printf("WARNING: couldn't get docker info -> %+v", err)
	}
	// Create a new table
	table := tablewriter.NewWriter(os.Stdout)
	// Set the table header
	table.SetHeader([]string{"NAME", "IMAGE", "STATUS"})
	// Iterate over the clusters and print the name, image and status
	for _, cluster := range clusters {
		// Get the container info
		containerInfo, _ := docker.ContainerInspect(context.Background(), cluster)
		// Add the cluster to the table
		clusterData := []string{cluster, containerInfo.Config.Image, containerInfo.ContainerJSONBase.State.Status}
		// If all is true, print all clusters, otherwise only print running clusters
		if containerInfo.ContainerJSONBase.State.Status == "running" || all {
			table.Append(clusterData)
		}
	}
	// Print the table
	table.Render()
}

// getClusters returns a list of cluster names which are folder names in the config directory
func getClusters() ([]string, error) {
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
		// checking if the file is a directory. as we are returning the names of the clusters, we only need to check for directories
		if file.IsDir() {
			clusters = append(clusters, file.Name())
		}
	}
	return clusters, nil
}
