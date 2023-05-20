//go:build integration
// +build integration

package test

import (
	"fmt"
	"github.com/cox96de/containervm/util"
	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/env"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestT(t *testing.T) {
	env.ChangeWorkingDir(t, "..")
	_ = run("docker", "rm", "vm")
	imagePath := "image.qcow2"
	_ = imagePath
	qemuCMD := fmt.Sprintf("qemu-system-x86_64 " +
		"-nodefaults " +
		"--nographic " +
		"-display none " +
		"-machine type=q35,usb=off " +
		// KVM is not enabled in GitHub Actions.
		//"--enable-kvm " +
		"-smp 4,sockets=1,cores=4,threads=1 " +
		"-m 4096M -device virtio-balloon-pci,id=balloon0 " +
		fmt.Sprintf("-drive file=%s,format=qcow2,if=virtio,aio=threads,media=disk,cache=unsafe,snapshot=on ", imagePath) +
		"-serial chardev:serial0 -chardev socket,id=serial0,path=/tmp/console.sock,server=on,wait=off " +
		"-vnc unix:/tmp/vnc.sock -device VGA " +
		"",
	)
	dockerRunCMD := "docker run --privileged " +
		"-v /tmp/containervm:/tmp " +
		"-v $PWD:/root " +
		"--name vm " +
		"-w /root " +
		"containervm " +
		"-- " +
		qemuCMD

	go func() {
		_ = run("bash", "-c", dockerRunCMD)
	}()
	defer func() {
		_ = run("docker", "stop", "vm")
		_ = run("docker", "rm", "vm")
	}()
	time.Sleep(time.Second * 3)
	ip, err := getContainerIP()
	assert.NilError(t, err)
	ip = strings.TrimRight(strings.TrimLeft(strings.TrimSpace(ip), "'"), "'")
	t.Logf("ip: %s", ip)
	testVM := func() error {
		output, err := util.Run("bash", "-c", "ping -c 1 "+ip)
		if err != nil {
			return err
		}
		t.Logf("output: %s", output)
		return nil
	}
	pass := false
	for i := 0; i < 20; i++ {
		err := testVM()
		if err == nil {
			pass = true
			break
		}
		t.Logf("error: %+v", err)
		time.Sleep(time.Second * 5)
	}
	assert.Assert(t, pass)
}

func getContainerIP() (string, error) {
	return util.Run("docker", "inspect", "-f", "'{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'", "vm")
}

func run(command string, args ...string) (err error) {
	fmt.Printf("run command: %s %+v\n", command, args)
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.WithMessage(err, "failed to run command")
	}
	if exitCode := cmd.ProcessState.ExitCode(); exitCode != 0 {
		return errors.Errorf("command exited with code %d", exitCode)
	}
	return nil
}
