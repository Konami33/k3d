package run

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func getDockerMachineIp() (string, error) {
	machine := os.ExpandEnv("$DOCKER_MACHINE_NAME")

	if machine == "" {
		return "", nil
	}

	// LookPath searches for an executable named file in the directories named by the PATH environment variable
	dockerMachinePath, err := exec.LookPath("docker-machine")
	if err != nil {
		return "", err
	}

	// Command returns the Cmd struct to execute the named program with the given arguments.
	// Output runs the command and returns its standard output.
	//ip is the IP address of a machine
	out, err := exec.Command(dockerMachinePath, "ip", machine).Output()

	//handle err
	if err != nil {
		log.Printf("Error executing 'docker-machine ip'")
		//ExitError is returned by the functions of the os package that can exit with a non-zero status.
		//Stderr returns the error stream returned by the command.
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Printf("%s", string(exitError.Stderr))
		}
		return "", err
	}

	//TrimSuffix returns s without the provided trailing suffix string. If s doesn't end with suffix, s is returned unchanged.
	ipStr := strings.TrimSuffix(string(out), "\n")
	ipStr = strings.TrimSuffix(ipStr, "\r")
	fmt.Printf("ipStr: %s\n", ipStr)
	return ipStr, nil
}
