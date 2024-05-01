package main

import (
	"fmt"
	"k3d-go/version"
	"log"
	"os"

	run "k3d-go/cli"

	"github.com/urfave/cli"
)

// defaultK3sImage specifies the default image being used for server and workers
const defaultK3sImage string = "docker.io/rancher/k3s"
const defaultK3sClusterName string = "k3s-default"

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
			// shell starts a shell in the context of a running cluster
			Name:  "shell",
			Usage: "Start a subshell for a cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: defaultK3sClusterName,
					Usage: "Set a name for the cluster",
				},
				// $ k3d bash -c 'kubectl cluster-info'
				cli.StringFlag{
					Name:  "command, c",
					Usage: "Run a shell command in the context of the cluster",
				},
				cli.StringFlag{
					Name:  "shell, s",
					Value: "auto",
					Usage: "which shell to use. One of [auto, bash, zsh]",
				},
			},
			Action: run.Shell,
		},
		{
			Name:    "create",
			Aliases: []string{"c"},
			Usage:   "Create a single- or multi-node k3s cluster in docker containers",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "name, n",
					Value: defaultK3sClusterName,
					Usage: "Set a name for the cluster",
				},
				// Most k3d arguments are using in "stringSlice" style, allowing the argument to supplied multiple times. Previously used string separated by ","
				cli.StringSliceFlag{
					Name:  "volume, v",
					Usage: "Mount one or more volumes into every node of the cluster (Docker notation: `source:destination`)",
				},
				// node specifier flags
				// usage: --publish 80:8080/tcp@worker-1
				cli.StringSliceFlag{
					Name:  "publish, add-port",
					Usage: "Publish k3s node ports to the host (Format: `[ip:][host-port:]container-port[/protocol]@node-specifier`, use multiple options to expose more ports)",
				},
				cli.IntFlag{
					Name:  "port-auto-offset",
					Value: 0,
					Usage: "Automatically add an offset (* worker number) to the chosen host port when using `--publish` to map the same container-port from multiple k3d workers to the host",
				},
				cli.StringFlag{
					Name: "version",
					//Value: version.GetK3sVersion(),
					Usage: "Choose the k3s image version",
				},
				//specify port
				cli.IntFlag{
					// TODO: only --api-port, -a soon since we want to use --port, -p for the --publish/--add-port functionality
					Name:  "api-port, a, port, p",
					Value: 6443,
					// Usage: "Map the Kubernetes ApiServer port to a local port (Note: --port/-p will have different functionality as of v2.0.0)",
					Usage: "Map the Kubernetes ApiServer port to a local port (Note: --port/-p will be used for arbitrary port mapping as of v2.0.0, use --api-port/-a instead for setting the api port)",
				},
				//specify timeout time
				cli.IntFlag{
					Name:  "timeout, t",
					Value: 0,
					Usage: "Set the timeout value when --wait flag is set (deprecated, use --wait <timeout> instead)",
				},
				//--wait flag
				cli.IntFlag{
					Name:  "wait, w",
					Value: 0,
					Usage: "Wait for the cluster to come up before returning until timoout (in seconds). Use --wait 0 to wait forever",
				},
				cli.StringFlag{
					Name:  "image, i",
					Usage: "Specify a k3s image (Format: <repo>/<image>:<tag>)",
					Value: fmt.Sprintf("%s:%s", defaultK3sImage, version.GetK3sVersion()),
				},
				//accept multiple string values. can be passed multiple values for a single flag.
				cli.StringSliceFlag{
					//name of the flag. can be used as either "--server-arg" or "-x"
					Name:  "server-arg, x",
					Usage: "Pass an additional argument to k3s server (new flag per argument)",
				},
				// environment variable
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
				//When creating clusters with the --auto-restart flag, any running cluster
				//will remain "running" up on docker daemon restart.
				cli.BoolFlag{
					Name:  "auto-restart",
					Usage: "Set docker's --restart=unless-stopped flag on the containers",
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
					Value: defaultK3sClusterName,
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
					Value: defaultK3sClusterName,
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
					Value: defaultK3sClusterName,
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
					Value: defaultK3sClusterName,
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
