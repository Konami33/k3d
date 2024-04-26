package main

import (
	"k3d-go/version"
	"log"
	"os"

	run "k3d-go/cli"

	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp() //creating a command line application
	app.Name = "k3d"
	app.Usage = "Run k3s in Docker!"
	app.Version = version.GetVersion()
	app.Authors = []cli.Author{
		{
			Name:  "Yasin",
			Email: "yasinarafat9889@gmail.com",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:    "check-tools",
			Aliases: []string{"ct"},
			Usage:   "Check if docker is running",
			Action:  run.CheckTools,
		},
		{
			Name:    "create",
			Aliases: []string{"c"},
			Usage:   "Create a single- or multi-node k3s cluster in docker containers",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "Set a name for the cluster",
				},
				cli.StringFlag{
					Name:  "volume, v",
					Usage: "Mount one or more volumes into every node of the cluster (Docker notation: `source:destination[,source:destination]`",
				},
				cli.StringFlag{
					Name: "version",
					//TO DO: add a function to automatically get the latest version
					Value: version.GetK3sVersion(),
					Usage: "Choose the k3s image version",
				},
				cli.IntFlag{
					Name:  "port, p",
					Value: 6443,
					Usage: "Map the Kubernetes ApiServer port to a local port",
				},
				cli.IntFlag{
					Name:  "timeout, t",
					Value: 0,
					Usage: "Set the timeout value when --wait flag is set",
				},
				cli.BoolFlag{
					Name:  "wait, w",
					Usage: "Wait for the cluster to come up before returning",
				},
				//accept multiple string values. can be passed multiple values for a single flag.
				cli.StringSliceFlag{
					//name of the flag. can be used as either "--server-arg" or "-x"
					Name:  "server-arg, x",
					Usage: "Pass an additional argument to k3s server (new flag per argument)",
				},
				cli.StringSliceFlag{
					Name:  "env, e",
					Usage: "Pass an additional environment variable (new flag per variable)",
				},
				//workder node
				cli.IntFlag{
					Name:  "workers",
					Value: 0,
					Usage: "Specify how many worker nodes you want to spawn",
				},
			},
			Action: run.CreateCluster,
		},
		{
			Name:    "delete",
			Aliases: []string{"d", "del"},
			Usage:   "Delete cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "Delete all existing clusters (this ignores the --name/-n flag)",
				},
			},
			Action: run.DeleteCluster,
		},
		{
			Name:  "stop",
			Usage: "Stop cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "Name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "Stop all running clusters (this ignores the --name/-n flag)",
				},
			},
			Action: run.StopCluster,
		},
		{
			Name:  "start",
			Usage: "Start a stopped cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "Name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "Start all stopped clusters (this ignores the --name/-n flag)",
				},
			},
			Action: run.StartCluster,
		},
		{
			Name:    "list",
			Aliases: []string{"ls", "l"},
			Usage:   "List all clusters",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "Also show non-running clusters",
				},
			},
			Action: run.ListClusters,
		},
		{
			Name:  "get-kubeconfig",
			Usage: "Get kubeconfig location for cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: "k3s_default",
					Usage: "Name of the cluster",
				},
				cli.BoolFlag{
					Name:  "all, a",
					Usage: "Get kubeconfig for all clusters (this ignores the --name/-n flag)",
				},
			},
			Action: run.GetKubeConfig,
		},
	}
	// global flags. Used in commands.go getKubeconfig function
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "Enable verbose output",
		},
	}
	err := app.Run(os.Args) //run the cli application
	if err != nil {
		log.Fatal(err)
	}
}
