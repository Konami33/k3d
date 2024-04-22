package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/urfave/cli"
)

// Command: [docker run --name k3s_default -e K3S_KUBECONFIG_OUTPUT=/output/kubeconfig.yaml --publish 6443:6443 --privileged -d rancher/k3s:v1.29.4-rc1-k3s1 server --https-listen-port 6443]
func createCluster(c *cli.Context) error {

	createClusterDir(c.String("name"))

	port := fmt.Sprintf("%s:%s", c.String("port"), c.String("port"))
	image := fmt.Sprintf("rancher/k3s:%s", c.String("version"))
	cmd := "docker"

	//required arguments
	args := []string{
		"run",
		"--name", c.String("name"),
		"-e", "K3S_KUBECONFIG_OUTPUT=/output/kubeconfig.yaml",
		"--publish", port,
		"--privileged",
	}

	//slice of string for any extra argument
	extraArgs := []string{}

	//check volume specific or not. append the extra argument --volume
	if c.IsSet("volume") {
		//extraArgs = append(extraArgs, fmt.Sprintf("--volume %s", c.String("volume")))
		extraArgs = append(extraArgs, "--volume", c.String("volume"))
	}
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
	}
	args = append(args,
		"-d",
		image,
		"server",
		"--https-listen-port", c.String("port"),
	)
	log.Printf("Creating cluster [%s]", c.String("name"))
	// build in function to run the command
	if err := run(true, cmd, args...); err != nil {
		log.Fatalf("FAILURE: couldn't create cluster [%s] --> %+v", c.String("name"), err)
		return err
	}
	log.Printf("SUCCESS: created cluster [%s]", c.String("name"))

	log.Printf(`You can now use the cluster with: 
	export KUBECONFIG="$(%s get-kubeconfig --name='%s')" 
	kubectl cluster-info`, os.Args[0], c.String("name"))
	return nil
}

// Command: docker rm -f Cluster_name
func deleteCluster(c *cli.Context) error {
	cmd := "docker"
	args := []string{"rm"}
	clusters := []string{}

	// operate on one or all clusters
	if !c.Bool("all") {
		// only one cluster deletion. Append only that cluster if exists
		clusters = append(clusters, c.String("name"))
	} else {
		// all cluster deletion
		clusterList, err := getClusterNames()
		if err != nil {
			log.Fatalf("ERROR: `--all` specified, but no clusters were found.")
		}
		// append all clusters to the list
		clusters = append(clusters, clusterList...)
	}

	//iterating over the cluster list
	for _, cluster := range clusters {
		log.Printf("Removing cluster [%s]", cluster)
		// adding cluster name to the list. Deleting one by one for more granular error handling
		args = append(args, cluster)
		if err := run(true, cmd, args...); err != nil {
			log.Printf("WARNING: couldn't delete cluster [%s], trying a force remove now.", cluster)
			// Removing the last element from the slice. (cluster name)
			args = args[:len(args)-1]
			// appending force flag and the cluster name
			args = append(args, "-f", cluster)
			// running the command again with -f flag
			if err := run(true, cmd, args...); err != nil {
				log.Printf("FAILURE: couldn't delete cluster [%s] -> %+v", cluster, err)
			}
			//after successfull deletion removing the last element. cluster name
			args = args[:len(args)-1]
		}
		deleteClusterDir(cluster)
		log.Printf("SUCCESS: removed cluster [%s]", cluster)
		args = args[:len(args)-1] // pop last element from list. -f flag
	}
	return nil
}

// Command: docker stop Cluster_name
func stopCluster(c *cli.Context) error {
	cmd := "docker"
	args := []string{"stop"}
	clusters := []string{}
	
	// if all is not specified then only stop the specific cluster
	if !c.Bool("all") {
		clusters = append(clusters, c.String("name"))
	} else { //otherwise all cluster
		clusterList, err := getClusterNames()
		if err != nil {
			log.Fatalf("ERROR: `--all` specified, but no clusters were found.")
		}
		clusters = append(clusters, clusterList...)
	}
	//iterating over the clusters
	for _, cluster := range clusters {
		log.Printf("Stopping cluster [%s]", cluster)
		args = append(args, cluster)
		// Running the docker command: docker stop cluster_name
		if err := run(true, cmd, args...); err != nil {
			log.Printf("FAILURE: couldn't stop cluster [%s] -> %+v", cluster, err)
		}
		log.Printf("SUCCESS: stopped cluster [%s]", cluster)
		args = args[:len(args)-1] // pop last element from list (name of last cluster)
	}
	// return nil to indicate success. No error.
	return nil
}

// Command: docker start Cluster_name
func startCluster(c *cli.Context) error {
	cmd := "docker"
	args := []string{"start"}
	clusters := []string{}

	// for one cluster
	if !c.Bool("all") {
		clusters = append(clusters, c.String("name"))
	} else {
		clusterList, err := getClusterNames()
		if err != nil {
			log.Fatalf("ERROR: `--all` specified, but no clusters were found.")
		}
		clusters = append(clusters, clusterList...)
	}
	for _, cluster := range clusters {
		log.Printf("Starting the cluster [%s]", cluster)
		args = append(args, cluster)

		if err := run(true, cmd, args...); err != nil {
			log.Printf("FAILURE: couldn't start cluster [%s] -> %+v", cluster, err)
		}
		log.Printf("SUCCESS: stopped cluster [%s]", cluster)
		args = args[:len(args)-1]
	}
	return nil
}

func listClusters(c *cli.Context) error {
	fmt.Println("TEST list")
	//listing all the cluster directories
	printClusters(c.Bool("all"))
	return nil
}

func getKubeConfig(c *cli.Context) error {
	//source and destination path
	sourcePath := fmt.Sprintf("%s:/output/kubeconfig.yaml", c.String("name"))
	destPath, _ := getClusterDir(c.String("name"))

	//command: docker cp
	cmd := "docker"
	args := []string{"cp", sourcePath, destPath}
	//exec.Command(cmd, args...).Args will return --> []string{"docker", "cp", "sourcePath", "destPath"}

	//executing command run()
	if err := run(true, cmd, args...); err != nil {
		log.Fatalf("FAILURE: couldn't get kubeconfig for cluster [%s] --> %+v", c.String("name"), err)
		return err
	}
	fmt.Printf("%s\n", path.Join(destPath, "Kubeconfig.yaml"))
	return nil
}

func main() {
	app := cli.NewApp() //creating a command line application

	//attributes
	app.Name = "k3d"
	app.Usage = "Run k3s in Docker!"
	app.Version = "v0.1.1"
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Yasin",
			Email: "yasinarafat9889@gmail.com",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:    "check-tools",
			Aliases: []string{"ct"},
			Usage:   "Check if docker is running",
			Action: func(c *cli.Context) error {
				log.Print("Checking docker...")
				cmd := "docker"
				args := []string{"version"}
				if err := run(true, cmd, args...); err != nil {
					log.Fatalf("Checking docker: FAILED")
					return err
				}
				log.Println("Checking docker: SUCCESS")
				return nil
			},
		},
		{
			Name:    "create",
			Aliases: []string{"c"},
			Usage:   "Create a single node k3s cluster in a container",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "Set a name for the cluster",
				},
				cli.StringFlag{
					Name:  "volume, v",
					Usage: "Mount a volume into the cluster node (Docker notation: `source:destination`",
				},
				cli.StringFlag{
					Name: "version",
					// Value:       "v0.1.0",
					Value: "v1.29.4-rc1-k3s1",
					Usage: "Choose the k3s image version",
				},
				cli.IntFlag{
					Name:  "port, p",
					Value: 6443,
					Usage: "Set a port on which the ApiServer will listen",
				},
			},
			Action: createCluster,
		},
		{
			Name:    "delete",
			Aliases: []string{"d"},
			Usage:   "Delete cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "delete all existing clusters (this ignores the --name/-n flag)",
				},
			},
			Action: deleteCluster,
		},
		{
			Name:  "stop",
			Usage: "Stop cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "name of the cluster",
				},
			},
			Action: stopCluster,
		},
		{
			Name:  "start",
			Usage: "Start a stopped cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "start all stopped clusters (this ignores the --name/-n flag)",
				},
			},
			Action: startCluster,
		},
		{
			Name:    "list",
			Aliases: []string{"ls", "l"},
			Usage:   "List all clusters",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "also show non-running clusters",
				},
			},
			Action: listClusters,
		},
		{
			Name:  "get-kubeconfig",
			Usage: "Get kubeconfig location for cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "get kubeconfig for all clusters (this ignores the --name/-n flag)",
				},
			},
			Action: getKubeConfig,
		},
	}

	err := app.Run(os.Args) //running the cli application
	if err != nil {
		log.Fatal(err)
	}
}

// function to run commands
func run(verbose bool, name string, args ...string) error {
	if verbose {
		log.Printf("Running command: %+v", append([]string{name}, args...))
	}
	// Create the command with the given arguments
	cmd := exec.Command(name, args...)
	// Set the command's output to be piped to the standard output
	cmd.Stdout = os.Stdout
	// Set the command's error output to be piped to the standard error
	cmd.Stderr = os.Stderr
	// Run the command
	return cmd.Run()
}
