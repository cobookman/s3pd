# S3 Parallel Downloader

CLI utility that downloads multiple s3 objects at a time, with multiple range-requests issued per object.
It also has support for copying between local filesystem locations using multiple threads.

Operations will always recurse the specified directories. When reading from a local filesystem, symlinks will not be followed.

Known Issues:
- If your S3 bucket has a folder and object with the same name, this utility will fail. (E.g. `s3://mybucket/test.txt` && `s3://mybucket/test.txt/another-object.txt`). This fails as POSIX filesystems cannot have a folder and file with the same absolute path.
- Support for writing to S3 has not yet been added.

### Benchmark resuts

**65.6069Gibps** - On a m5n.24xl, transferring 65GiB of data from S3 (2,080 x 32MiB objects) across 185*2 concurrent HTTP requests
```
./s3pd-linux-amd64 \
--region=us-west-2 \
--workers=185 \
--threads=2 \
--partsize=$((4*1024*1024)) \
s3://test-400gbps-s3/32MiB/ /mnt/ram-disk
```

**73.9592Gibps** - On a m5n.24xl, transferring 292GiB of data from S3 (146 x 2GiB objects) across 40*32 concurrent HTTP requests
```
./s3pd-linux-amd64 \
--region=us-west-2 \
--workers=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
s3://test-400gbps-s3/2GiB/ /mnt/ram-disk
```

**101.3641Gibps** - On a dl1.24xl, with 4x100 ENIs transferring 292GiB of data from S3 (146 x 2GiB objects) across 40*32 concurrent HTTP requests
```
./s3pd-linux-amd64 \
--region=us-west-2\
--workers=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
--nics=en0,en1,en2,en3
s3://test-400gbps-s3/2GiB/ /mnt/ram-disk
```

**258.0953Gibps** - On a m5n.24xl, transferring 65GiB of data from a **local RAM disk to a local RAM disk** across 300 concurrent writers
```
./s3pd-linux-amd64 \
--workers=300 \
--threads=1 \
--partsize=$((128*1024)) \
/mnt/ram-disk/32MiB /mnt/ram-disk/234
```


### Example CLI usage
Equivalent to: `aws s3 cp s3://ml-training-dataset/pictures/* /mnt/nvme-local-disks`
But instead of downloading 1 object at a time, it'll download 40 objects at a time, with a higher concurrency rate than the aws s3 utility.

```
./s3pd-linux-amd64 \
--region=us-west-2 \
--workers=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
s3://ml-training-dataset/pictures /mnt/my-nvme-local-disks
```

If you just want to run a benchmark, and avoid needing to spin up a large-enough RAM disk, you can use the `--benchmark` flag which will only store the data temporarily in an in-memory buffer. For example:
```
./s3pd-linux-amd64 \
--workers=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
--benchmark \
s3://test-400gbps-s3/2GiB/
```

If you want to copy between the local filesystem 
```
./s3pd-linux-amd64 \
--workers=40 \
--threads=32 \
--partsize=$((8*1024*1024)) \
/mnt/my-nvme-disk-1/datasetA /mnt/my-nvme-disk-2/
```

### Multicard with p4d.24xl & dl1.24xl
p4d and dl1 ec2 instances offer 4x100Gibps of throughput. This is accomplished by attaching 4 ENIs 
to these instances each with its own distinct `NetworkCardIndex`. Linux determines which network interface 
to send traffic to based upon its local subnet and routing rules. This means that traffic to the same IP 
address will by default go through the same network interface.

By using the `--nics` flag we can configure our HTTP clients to distribute the HTTP requests across each of the specified NICs. 

#### Getting the Network Interface name

You can use the command `ifconfig` or `ip a` to list all the network interfaces currently attached to your linux instance. 
The naming convention depends on the Linux Distribution & Linux configuration. Generally the ENIs will have names
in the format of `en0` or `eth0`.

#### S3pd with multiple NICs

Simply add the `--nics` flag with a list of network interfaces (ENIs) you'd like HTTP traffic to be round-robin load balanced across.
For example this command load balances traffic across 4 ENIs attached at `en0`, `en1`, `en2`, and `en3`.

```
./s3pd-linux-amd64 \
--region=us-west-2\
--workers=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
--nics=en0,en1,en2,en3
s3://test-400gbps-s3/2GiB/ /mnt/ram-disk
```

#### Spinning up a multicard Ec2 instance 
**Note** most ec2 instance types do not support the 4x100G configuration.
In particular the p4d and dl1 support this.

> When you add a second network interface, Ec2 will no longer auto-assign a public IPv4 address.
You will not be able to connect to the instance over IPv4 unless you assign an Elastic IP address to the primary
network interface (eth0). You can assign the Elastic IP address after you complete the Launch wizard.
[(AWS Docs)](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/MultipleIP.html#assignIP-launch)


The following script/cli command will create a dl1.24xl with 4 ENIs. Where each
ENI is attached to a different NetworkCardIndex & DeviceIndex, giving 4x100G of throughput.
When traffic is sharded across all 4 of the ENIs, you can get up-to 400G of network throughput.

```
#!/bin/bash
SUBNET="subnet-ddddaab" # Replace with the target subnet-id
SG="sg-000000000000ddddf # Replace with the Security Group ID you'd like to assign
AMI_ID="ami-00f7e5c52c0f43726" # Replace with the AMI you'd to have this instance use
INSTANCE_TYPE="dl1.24xlarge"

aws ec2 run-instances --region us-west-2 \
	--image-id $AMI_ID \
	--instance-type $INSTANCE_TYPE \
	--key-name m1-mbp-work \
        --network-interfaces "NetworkCardIndex=0,DeviceIndex=0,Groups=${SG},SubnetId=${SUBNET},InterfaceType=efa" \
                             "NetworkCardIndex=1,DeviceIndex=1,Groups=${SG},SubnetId=${SUBNET},InterfaceType=efa" \
                             "NetworkCardIndex=2,DeviceIndex=2,Groups=${SG},SubnetId=${SUBNET},InterfaceType=efa" \
                             "NetworkCardIndex=3,DeviceIndex=3,Groups=${SG},SubnetId=${SUBNET},InterfaceType=efa"
```
