package core

import "os/exec"

func execLookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}
