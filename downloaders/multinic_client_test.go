package downloaders

import (
	"io/ioutil"
    "testing"
)

func TestNewMultiNicHTTPClient(t *testing.T) {
	_, err := NewMultiNicHTTPClient([]string{"en0"})
	if err != nil {
		t.Error(err)
	}
}

func TestMakeClient(t *testing.T) {
	mn, err := NewMultiNicHTTPClient([]string{"en0"})
	if err != nil {
		t.Error(err)
	}
	_, err = mn.Client()
	if err != nil {
		t.Error(err)
	}
}

func TestMakeHTTPCall(t *testing.T) {
	mn, err := NewMultiNicHTTPClient([]string{"en0"})
	if err != nil {
		t.Error(err)
	}
	
	client, err := mn.Client()
	if err != nil {
		t.Error(err)
	}

	// http request
	response, err := client.Get("https://api.ipify.org/") // get my IP address
	if err != nil {
		t.Error(err)
	}

	data, err := ioutil.ReadAll(response.Body);
	if err != nil {
		t.Error(err)
	}

	if len(string(data)) == 0 {
		t.Error("No data retrieved from HTTP Requeest")
	}
}

func TestMakeClientSpeed(t *testing.T) {
	br := testing.Benchmark(func(b *testing.B) {
		mn, err := NewMultiNicHTTPClient([]string{"en0"})
		if err != nil {
			b.Error(err)
		}
	
		_, err = mn.Client()
		if err != nil {
			b.Error(err)
		}
	})
	t.Logf("Time it took to create new HTTP Client: %s\n", br)
}