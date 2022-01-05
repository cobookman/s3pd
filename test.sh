go run *.go \
--partsize=$((1*1024)) \
--workers=4 \
--threads=2 \
--loglevel=debug \
--nics=en0 \
s3://test-400gbps-s3/1.bin \
--benchmark
