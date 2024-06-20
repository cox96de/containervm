# containervm

This project provides a lightweight solution for running QEMU VMs in a Kubernetes pod without any additional Kubernetes
modifications.

This project is inspired by [BBVA/kvm@GitHub](https://github.com/BBVA/kvm/blob/master/startvm)

## Usage/Examples

### Prepare a QEMU image

Download a prebuilt QEMU image from https://cloud.debian.org/images/cloud/.

### Build Docker image:

Build the Docker image by executing the following command:

```shell
docker build -t containervm .
```

### Run Image:

Run the image with the following command:

```shell
docker run \
--privileged \
--rm \
-v /tmp/containervm:/tmp \
-v $PWD:/root \
--name vm \
-w /root \
containervm \
-- \
qemu-system-x86_64 \
-nodefaults \
--nographic \
-display none \
-machine type=q35,usb=off \
-smp 4,sockets=1,cores=4,threads=1 \
-m 1024M -device virtio-balloon-pci,id=balloon0 \
-drive file=debian-11-genericcloud-amd64-20230515-1381.qcow2,format=qcow2,if=virtio,aio=threads,media=disk,cache=unsafe,snapshot=on \
-serial chardev:serial0 -chardev socket,id=serial0,path=/tmp/console.sock,server=on,wait=off \
-vnc unix:/tmp/vnc.sock \
-device VGA
```

### Obtain Docker's IP

To obtain Docker's IP, execute the following command:

```shell
docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' vm
```

The output will be the IP address of Docker, such as 172.17.0.2.

### Connect to the VM with IP

Make sure the SSH server is enabled, and execute the following command to connect to the VM with IP:

```shell
ssh root@172.17.0.2
```

### Use VNC to connect to the VM

If the VM you downloaded disables SSH for security reasons, you can use VNC to connect to the VM. Mount the VNC socket
outside the container with the path /tmp/containervm/vnc.sock. Convert it to a TCP connection, and then use a VNC client
to connect to localhost:5000.

To do this, execute the following two commands:

```shell
socat TCP-LISTEN:5000,reuseaddr,fork UNIX-CLIENT:/tmp/containervm/vnc.sock
```

```shell
vncviewer localhost:5000
```


### Use minicom to connect to the VM's serial port
```shell
minicom -D unix\#/tmp/containervm/console.sock
```