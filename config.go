package main

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/mitchellh/go-homedir"
)

// createDirIfNotExists checks for the existence of a directory and creates it along with all required parents if not.
// It returns an error if the directory (or parents) couldn't be created and nil if it worked fine or if the path already exists.
func createDirIfNotExists(path string) error {
	// check if the path already exists
	// os.Stat(path) is used to retrive info about a file or directory about the path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// if the path doesn't exist, create it
		// os.MkdirAll(path, os.ModePerm) creates the directory and all required parents
		return os.MkdirAll(path, os.ModePerm)
	}
	return nil
}

// createClusterDir creates a directory with the cluster name under $HOME/.config/k3d/<cluster_name>.
// The cluster directory will be used e.g. to store the kubeconfig file.
func createClusterDir(name string) {
	// get the cluster directory path
	clusterPath, _ := getClusterDir(name)
	// create the cluster directory if it doesn't exist
	if err := createDirIfNotExists(clusterPath); err != nil {
		log.Fatalf("ERROR: couldn't create cluster directory [%s] -> %+v", clusterPath, err)
	}

}

// deleteClusterDir contrary to createClusterDir, this deletes the cluster directory under $HOME/.config/k3d/<cluster_name>
func deleteClusterDir(name string) {
	clusterPath, _ := getClusterDir(name)
	// delete the cluster directory
	if err := os.RemoveAll(clusterPath); err != nil {
		log.Printf("WARNING: couldn't delete cluster directory [%s]. You might want to delete it manually.", clusterPath)
	}
}

// getClusterDir returns the path to the cluster directory which is $HOME/.config/k3d/<cluster_name>
func getClusterDir(name string) (string, error) {
	// get the user's home directory
	homeDir, err := homedir.Dir()
	// if the user's home directory couldn't be detected, return an error
	if err != nil {
		log.Printf("ERROR: Couldn't get user's home directory")
		return "", err
	}
	// return the path to the cluster directory
	return path.Join(homeDir, ".config", "k3d", name), nil
}

// listClusterDirs prints the names of the directories in the config folder (which should be the existing clusters)
func listClusterDirs() {
	homeDir, err := homedir.Dir()  //detecting the user's home directory
	// if the user's home directory couldn't be detected, return an error
	if err != nil {
		log.Fatalf("ERROR: Couldn't get user's home directory")
	}
	// get the path to the config directory
	configDir := path.Join(homeDir, ".config", "k3d")
	// read the files in the config directory
	files, err := os.ReadDir(configDir)
	// if the config directory couldn't be read, return an error
	if err != nil {
		log.Fatalf("ERROR: Couldn't list files in [%s]", configDir)
	}
	// print the names of the directories in the config directory
	fmt.Println("NAME")
	// iterate over the files in the config directory
	for _, file := range files {
		if file.IsDir() {
			fmt.Println(file.Name())
		}
	}
}
