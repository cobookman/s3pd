package main

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

type s3PathTest struct {
	path, expectedBucket, expectedPrefix string
}

var s3PathTests = []s3PathTest{
	s3PathTest{"s3://my-bucket/my-folder", "my-bucket", "my-folder"},
	s3PathTest{"s3://my-bucket/my-folder/", "my-bucket", "my-folder/"},
	s3PathTest{"s3://my-bucket/", "my-bucket", ""},
	s3PathTest{"s3://my-bucket", "my-bucket", ""},
	s3PathTest{"s3://my-bucket//", "my-bucket", "/"},
	s3PathTest{"http://my-bucket", "", ""},
	s3PathTest{"my-bucket", "", ""},
	s3PathTest{"s3:my-bucket", "", ""},
	s3PathTest{"s3:/my-bucket", "", ""},
}

func TestParseS3Path(t *testing.T) {
	for _, test := range s3PathTests {
		bucket, prefix := parseS3Path(test.path)
		if bucket != test.expectedBucket {
			t.Errorf("Bucket %q not equal to expected: %q", bucket, test.expectedBucket)
		}
		if prefix != test.expectedPrefix {
			t.Errorf("Prefix %q not equal to expected: %q", prefix, test.expectedPrefix)
		}
	}
}

func TestGetDownloader(t *testing.T) {
	// Test for S3 download
	s3c, err := NewConfig([]string{"s3pd", "s3://mybucket/prefix", "/mnt/ram-disk/"})
	assert.Equal(t, nil, err, "NewConfig should not return an error for valid syntax")

	s3downloader, err := getDownloader(s3c, nil, nil)
	assert.Equal(t, nil, err, "Getting the downloader should not have an error")
	assert.Equal(t, "*downloaders.S3Download", reflect.TypeOf(s3downloader).String(),
		"downloader should be of right type")

	// Test for Filesystem to filesystem copy
	fsc, err := NewConfig([]string{"s3pd", "/mnt/path1/", "/mnt/path2/"})
	assert.Equal(t, nil, err, "NewConfig should not return an error for valid syntax")

	fsDownloader, err := getDownloader(fsc, nil, nil)
	assert.Equal(t, nil, err, "Getting the downloader should not have an error")
	assert.Equal(t, "*downloaders.FilesystemDownload", reflect.TypeOf(fsDownloader).String(),
		"downloader should be of right type")

}
