package run

import (
	"log"
	"os"
	"os/exec"
)

// runCommand accepts the name and args and runs the specified command
func runCommand(verbose bool, name string, args ...string) error {
	if verbose {
		log.Printf("Running command: %+v", append([]string{name}, args...))
	}
	// Create the command with the specified name and args
	cmd := exec.Command(name, args...)
	// Set the output to be the same as the current process
	cmd.Stdout = os.Stdout
	// Set the error output to be the same as the current process
	cmd.Stderr = os.Stderr
	// Run the command
	return cmd.Run()
}