package main

import (
	"fmt"
	"reflect"
    "testing"
	"github.com/stretchr/testify/assert"
)

type configTest struct {
	args []string
	expected Config
}

var defaults = Config{
	source: "",
	destination: "",
	region: "",
	workers: 10,
	threads: 5,
	partsize: 5*1024*1024,
	maxList: 1000,
	nics: "",
	isBenchmark: false,
	loglevel: "NOTICE",
	cpuprofile: "",		
}


var configTests []configTest
func TestMain(m *testing.M) {
	test1 := defaults
	test1.source = "s3://mybucket/prefix"
	test1.destination = "/mnt/ram-disk/"
	configTests = append(configTests, configTest{
		args: []string{"s3pd", "s3://mybucket/prefix", "/mnt/ram-disk/"},
		expected: test1,
	})

	test2 := defaults
	test2.source = "s3://mybucket/prefix/longer//"
	test2.destination = "/mnt/ram-disk"
	test2.region = "us-west-2"
	test2.workers = 100
	test2.threads = 20
	test2.partsize = 1 * 1024 * 1024
	test2.nics = "en0,en1,en2,en3"
	test2.loglevel = "DEBUG"
	test2.cpuprofile = "~/prof.prof"
	configTests = append(configTests, configTest{
		args: []string{"s3pd", 
			"s3://mybucket/prefix/longer//", "/mnt/ram-disk",
			"--region=us-west-2",
			"--workers=100",
			"--threads=20",
			"--partsize=1048576",
			"--nics=en0,en1,en2,en3",
			"--loglevel=DEBUG",
			"--cpuprofile=~/prof.prof"},
		expected: test2,
	})

	test3 := defaults
	test3.source = "s3://mybucket/prefix/longer"
	test3.isBenchmark = true
	configTests = append(configTests, configTest{
		args: []string{"s3pd", 
			"s3://mybucket/prefix/longer", "/mnt/ram-disk",
			"--benchmark"},
		expected: test3,
	})

	test4 := defaults
	test4.source = "s3://mybucket/prefix/longer/"
	test4.destination = "/mnt/ram-disk"
	test4.region = "us-west-2"
	test4.threads = 20
	configTests = append(configTests, configTest{
		args: []string{"s3pd", 
			"--region=us-west-2",
			"s3://mybucket/prefix/longer/", "/mnt/ram-disk",
			"--threads=20"},
		expected: test4,
	})
	m.Run()
}

func TestNewConfig(t *testing.T) {
	
	fmt.Printf("Expected configTests[0].expected.workers: %v\n", configTests[0].expected.workers)
	for _, ct := range configTests {
		actual, err := NewConfig(ct.args)
		if err != nil {
			t.Error(err)
		}

		assert.True(t, reflect.DeepEqual(*actual, ct.expected))
	}
}

func TestNicsArr(t *testing.T) {
	var c Config
	var arr []string
	c = Config{nics: ""}
	assert.Equal(t, 0, len(c.NicsArr()), "Should have length of zero")

	c = Config{nics: "en0,en1"}
	arr = c.NicsArr()
	assert.Equal(t, "en0", arr[0], "Should parse string")
	assert.Equal(t, "en1", arr[1], "Should parse string")


	c = Config{nics: "en1,"}
	arr = c.NicsArr()
	assert.Equal(t, "en1", arr[0], "Should remove trailing comman")
	assert.Equal(t, 1, len(arr), "Should remove trailing comman")
}