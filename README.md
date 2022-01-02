# S3 Parallel Downloader

CLI utility that downloads multiple s3 objects at a time, with multiple range-requests issued per object


### Example benchmark for downloading lots of 32MiB files
This will download 185 objects at a time.
When downloading an object, it'll use 4 threads each downloading a 4MiB chunk of the file.

Aka 185 * 4 total threads in use, and 185 * 4 concurrent HTTP requests to S3.
.
```
./bin/s3pd-linux-amd64 \
--bucket=test-400gbps-s3 \
--region=us-west-2 \
--prefix=32MiB/ \
--downloaders=185 \
--threads=4 \
--partsize=$((4*1024*1024)) \
--loglevel=ERROR \
--benchmark
```

### Example benchmark for downloading lots of 2GiB files
This will have 40 objects downloaded at a time.
When downloading an object, it'll use 32 threads each downloading 16MiB chunks of the file.

Aka 40 * 32 total threads in use, and 40 * 32 concurrent HTTP requests to s3
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


### Benchmark resuts

**65.6069Gibps** - 7926ms transfering 65GiB of data - downloaded  2,080 32MiB objects
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

**73.9592Gibps** - 31585ms transfering 292GiB of data - downloaded 146 2GiB objects
./s3pd-linux-amd64 \
--bucket=test-400gbps-s3 \
--region=us-west-2 \
--writedir=/mnt/ram-disk \
--prefix=2GiB/ \
--downloaders=40 \
--threads=32 \
--partsize=$((16*1024*1024)) (edited)



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


