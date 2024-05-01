package run

import (
	"fmt"
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
	out, err := exec.Command(dockerMachinePath, "ip").Output()

	//TrimSuffix returns s without the provided trailing suffix string. If s doesn't end with suffix, s is returned unchanged.
	ipStr := strings.TrimSuffix(string(out), "\n")
	ipStr = strings.TrimSuffix(ipStr, "\r")
	fmt.Printf("ipStr: %s\n", ipStr)
	return ipStr, err
}
