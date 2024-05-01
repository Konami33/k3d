package run

import (
	"context"
	"fmt"
	"log"

	"github.com/docker/docker/api/types/filters"

	"github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"
)
//add k3d prefix to every container name
func k3dNetworkName(clusterName string) string {
	return fmt.Sprintf("k3d-%s", clusterName)
}

func createClusterNetwork(clusterName string) (string, error) {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}

	// check if there is any netork found. if found take the first one
	args := filters.NewArgs()
	args.Add("label", "app=k3d")
	args.Add("label", "cluster="+clusterName)
	// NetworkList returns the list of networks configured in the docker host. returns []types.NetworkResource
	nl, err := docker.NetworkList(ctx, types.NetworkListOptions{Filters: args})
	if err != nil {
		return "", fmt.Errorf("failed to list networks\n%+v", err)
	}

	if len(nl) > 1 {
		log.Printf("WARNING: Found %d networks for %s when we only expect 1\n", len(nl), clusterName)
	}

	// if any network found return the first one
	if len(nl) > 0 {
		return nl[0].ID, nil
	}
	
	// resp: containens the info about the newly created network, such as its ID, name, and configuration.
	// create the network with a set of labels and the cluster name as network name
	resp, err := docker.NetworkCreate(ctx, k3dNetworkName(clusterName), types.NetworkCreate{
		// "app": "k3d": indicates that the network is associated with the "k3d" application.
		// "cluster" : clusterName: indicates the name of the network
		Labels: map[string]string{
			"app":     "k3d",
			"cluster": clusterName,
		},
	})
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't create network\n%+v", err)
	}

	return resp.ID, nil
}

func deleteClusterNetwork(clusterName string) error {
	ctx := context.Background()
	docker, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv)
	if err != nil {
		return fmt.Errorf("ERROR: couldn't create docker client\n%+v", err)
	}


	//This code block performs a filtered listing of Docker networks using specific label-based criteria --> 
	// app=k3d
	// cluster=clusterName
	filters := filters.NewArgs()
	filters.Add("label", "app=k3d")
	filters.Add("label", fmt.Sprintf("cluster=%s", clusterName))

	// NetworkList returns the list of networks configured in the docker host. (filtered)
	networks, err := docker.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("ERROR: couldn't find network for cluster %s\n%+v", clusterName, err)
	}

	for _, network := range networks {
		// NetworkRemove removes an existent network from the docker host.
		if err := docker.NetworkRemove(ctx, network.ID); err != nil {
			log.Printf("WARNING: couldn't remove network for cluster %s\n%+v", clusterName, err)
			continue
		}
	}
	return nil
}
