package dns

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

const wantName = "testhost.testdomain"

func Test_ExampleCustomResolver(t *testing.T) {
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		fmt.Fprint(res, "Hello World!")
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	go http.Serve(listener, nil)

	f := &MemResolver{
		LookupIP: myLookupIP,
	}
	// override lookupIP
	resolver := NewMemoryResolver(f)

	// use the new resolver for an http connection
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Resolver:  resolver,
		}).DialContext,
	}

	client := &http.Client{
		Transport: tr,
	}

	// Connect directly to the listener
	url := "http://" + listener.Addr().String()
	if err := connect(client, url); err != nil {
		panic(err)
	}

	// Connect to the custom domain and check it redirects to localhost
	url = "http://" + wantName + ":" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := connect(client, url); err != nil {
		panic(err)
	}

	// Connect to an external server
	url = "http://www.google.es"
	if err := connect(client, url); err != nil {
		panic(err)
	}

	// Output:
	// Got response 200 from http://127.0.0.1:44997: Hello World!
	// Got response 200 from http://testhost.testdomain:44997: Hello World!
}

func myLookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	// fqdn appends a dot
	if wantName == strings.TrimSuffix(host, ".") {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	return net.DefaultResolver.LookupIP(ctx, network, host)

}

func connect(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("Failed get: %s", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf(
		"Got response %d from %s: %s\n",
		resp.StatusCode, url, string(body)[:12])
	return nil
}
