# S3 Parallel Downloader

CLI utility that downloads multiple s3 objects at a time, with multiple range-requests issued per object


### Benchmark resuts

**65.6069Gibps** - 7926ms transfering 65GiB of data - downloaded  2,080 32MiB objects across 370 (185 * 2) concurrent HTTP requests
```
./s3pd-linux-amd64 \
--bucket=test-400gbps-s3 \
--region=us-west-2 \
--writedir=/mnt/ram-disk \
--prefix=32MiB/ \
--downloaders=185 \
--threads=2 \
--partsize=$((4*1024*1024))
```

**73.9592Gibps** - 31585ms transfering 292GiB of data - downloaded 146 2GiB objects across 1,280 (40*32) concurrent HTTP requests
```
./s3pd-linux-amd64 \
--bucket=test-400gbps-s3 \
--region=us-west-2 \
--writedir=/mnt/ram-disk \
--prefix=2GiB/ \
--downloaders=40 \
--threads=32 \
--partsize=$((16*1024*1024)) 
```


### Example CLI usage
Equivalent to: `aws s3 cp s3://ml-training-dataset/pictures/* /mnt/nvme-local-disks`
But instead of downloading 1 object at a time, it'll download 40 objects at a time, with a higher concurrency rate than the aws s3 utility.

```
./s3pd-linux-amd64 \
--bucket=ml-training-dataset \
--region=us-west-2 \
--prefix=pictures/ \
--downloaders=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
--writedir=/mnt/my-nvme-local-disks 
```

If you just want to run a benchmark, and avoid needing to spin up a large-enough RAM disk, you can use the `--benchmark` flag which will only store the data temporarily in an in-memory buffer. For example:
```
./s3pd-linux-amd64 \
--bucket=test-400gbps-s3 \
--region=us-west-2 \
--prefix=2GiB/ \
--downloaders=40 \
--threads=32 \
--partsize=$((16*1024*1024)) \
--benchmark
```


