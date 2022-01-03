# S3 Parallel Downloader

CLI utility that downloads multiple s3 objects at a time, with multiple range-requests issued per object.
Also supports copying between filesystem locations using multiple threads

Copy operations will always recurse the directory. When reading from a local filesystem, symlinks will not be followed


### Benchmark resuts

**65.6069Gibps** - 7926ms transfering 65GiB of data - downloaded  2,080 32MiB objects across 370 (185 * 2) concurrent HTTP requests
```
./s3pd-linux-amd64 \
--region=us-west-2 \
--prefix=32MiB/ \
--workers=185 \
--threads=2 \
--partsize=$((4*1024*1024))
s3://test-400gbps-s3/32MiB/ /mnt/ram-disk
```

**73.9592Gibps** - 31585ms transfering 292GiB of data - downloaded 146 2GiB objects across 1,280 (40*32) concurrent HTTP requests
```
./s3pd-linux-amd64 \
--region=us-west-2 \
--workers=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
s3://test-400gbps-s3/2GiB/ /mnt/ram-disk
```

**249.5305Gibps** - 1994ms transferring 65GiB of data from a **local RAM disk to a local RAM disk**
```
./s3pd-linux-amd64 \
--workers=100 \
--threads=2 \
--partsize=$((64*1024)) \
/mnt/ram-disk/32MiB /mnt/ram-disk/scratch
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
