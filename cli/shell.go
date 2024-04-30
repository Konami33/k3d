package run

import (
	"fmt"
	"os"
	"os/exec"
)

func bashShell(cluster string, command string) error {
	kubeConfigPath, err := getKubeConfig(cluster)
	if err != nil {
		return err
	}

	// ExpandEnv replaces ${var} or $var in the string according to the values of the current environment variables. References to undefined variables are replaced by the empty string.
	//this code prevents the execution of further actions that would start a new subshell of a k3d cluster if the current shell session is already in a subshell of a k3d cluster, ensuring that the user does not unintentionally create nested subshells.
	subShell := os.ExpandEnv("$__K3D_CLUSTER__")
	if len(subShell) > 0 {
		return fmt.Errorf("[ERROR]: Already in subshell of cluster %s", subShell)
	}

	// find out the bash path
	// LookPath searches for an executable named file in the directories named by the $PATH environment variable. LookPath also uses $PATHEXT environment variable to match a suitable candidate.
	// If a match is found, LookPath returns the absolute pathname of the executable file.
	// If no match is found, LookPath returns the string "", and err is set to os.ErrNotExist.
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return err
	}

	//"/bin/bash": Path to the Bash executable. Specifies that the command to be executed is Bash.
	// "--noprofile": an argument passed to Bash. It instructs Bash not to read the system-wide profile file for login shells. Useful when you want to start Bash quickly without loading any additional configurations.
	// "--norc": It instructs Bash not to read the user's ~/.bashrc file. Similar to --noprofile, it helps start Bash more quickly without loading additional configurations.
	// Command returns the Cmd struct to execute the named program with the given arguments. It sets only the Path and Args in the returned structure.

	cmd := exec.Command(bashPath, "--noprofile", "--norc")

	if len(command) > 0 {
		//  k3d bash -c 'kubectl cluster-info'
		cmd.Args = append(cmd.Args, "-c", command)
	}

	// Set up stdio
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	// Set up Prompt
	//Getenv retrieves the value of the environment variable named by the key.
	// In Bash, PS1 is an environment variable that defines the format of the primary prompt displayed to the user. Includes information such as the username, hostname, current directory, and other relevant details.
	// "PS1=\[%s}%s": Format of the string. Sets PS1 to a custom value. The \[ and \] are escape sequences in Bash that denote non-printing characters, which is often used for colorizing the prompt.
	// The resulting prompt will display the cluster name alongside the existing prompt string.
	// see more: https://linuxsimply.com/bash-scripting-tutorial/variables/types/ps1/
	setPS1 := fmt.Sprintf("PS1=[%s}%s", cluster, os.Getenv("PS1"))

	// Set up KUBECONFIG
	setKube := fmt.Sprintf("KUBECONFIG=%s", kubeConfigPath)
	// creating an environment variable __K3D_CLUSTER__=cluster
	subShell = fmt.Sprintf("__K3D_CLUSTER__=%s", cluster)
	// Environ returns a copy of strings representing the environment, in the form "key=value".
	// adding the environment variables to the newEnv
	newEnv := append(os.Environ(), setPS1, setKube, subShell)
	// Set up environment of the cmd
	cmd.Env = newEnv

	return cmd.Run()
}