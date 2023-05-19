package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"os"
	"os/exec"
)

func main() {
	pflag.Parse()
	args := pflag.Args()
	qemuCMD := exec.Command(args[0], args[1:]...)
	qemuCMD.Stdin = os.Stdin
	qemuCMD.Stdout = os.Stdout
	qemuCMD.Stderr = os.Stderr
	if err := qemuCMD.Start(); err != nil {
		log.Fatalf("failed to start qemu: %+v", err)
	}
	if err := qemuCMD.Wait(); err != nil {
		log.Fatalf("failed to wait for qemu: %+v", err)
	}
	log.Infof("qemu exited with code %d", qemuCMD.ProcessState.ExitCode())
}
