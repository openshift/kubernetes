# Kubernetes API Server resolver

Kubernetes conformant cluster must have a service named "kubernetes" on the
default namespace referencing the API servers. The "kubernetes.default" service
must have Endpoints and EndpointSlices pointing to each API server instance.

The Kubernetes API server resolver polls periodically the API server to refresh
the local cache with the IPs present in the published endpoints. The resolver
will return first the IPs that are present in the same node, to favor local
connections.

The resolver will fall back to the default golang network resolver if:
- It's not able to obtain the API server Endpoints.
- The configured API server hostname is not resolvable via DNS, per example, is
  an IP address or is resolved via /etc/hosts.
- The configured API server URL has a different port than the one used in the
  Endpoints.

## Configuration

The resolver has two options:

* period: defines the period of time for polling the API server and refresh the
  cache with the IPs obtained from the Endpoints object.
* timeout: defines the maximum amount of time the resolver will be trying to use
  the cache IPs under network failures, once expired it will fall back to the
  configured API server hostname and clean the cache.

## How to use it

The API server resolver uses the same configuration than client-go, in order to
use it we need to instantiate it with a valid configuration to obtain a new
net.Resolver. Then, we can add this net.Resolver to the configuration to create
a new clientset with our custom Resolver.

NOTE: The API Server resolver doesn't works if the configuration is using a
custom Dial function, since this will override it.

```go
    // use the current context in kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
    if err != nil {
        panic(err.Error())
    }
    // create an API server resolver
    r, err := NewResolver(ctx, config)
    if err != nil {
        return nil, err
    }
    
    // modify the configuration to use the resolver
    config.Resolver = r

    // create the clientset
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err.Error())
    }
```

## How it works

### Golang net/http

The default HTTP client in Go resolves domain names indirectly with the Dial
function. https://pkg.go.dev/net#hdr-Name_Resolution

Looking at the HTTP implementation, from top to bottom: (There are multiple
Dial, DialTLS, DialContext, use Dial to simplify)

http.Client -> http.Transport -> Dial 

This Dial function is implemented by default by net.Dialer, that allows to
specify an optional Resolver:

https://pkg.go.dev/net#Dialer
```go
type Dialer struct {
...
    // Resolver optionally specifies an alternate resolver to use.
    Resolver *Resolver
}
```

and this Resolver has another Dial function inside
https://pkg.go.dev/net#Resolver
```go
type Resolver struct {
...
    // Dial optionally specifies an alternate dialer for use by
    // Go's built-in DNS resolver to make TCP and UDP connections
    // to DNS services. The host in the address parameter will
    // always be a literal IP address and not a host name, and the
    // port in the address parameter will be a literal port number
    // and not a service name.
    // If the Conn returned is also a PacketConn, sent and received DNS
    // messages must adhere to RFC 1035 section 4.2.1, "UDP usage".
    // Otherwise, DNS messages transmitted over Conn must adhere
    // to RFC 7766 section 5, "Transport Protocol Selection".
    // If nil, the default dialer is used.
    Dial func(ctx context.Context, network, address string) (Conn, error)
}
```


### RESTClient

RESTClient imposes common Kubernetes API conventions on a set of resource paths.
The baseURL is expected to point to an HTTP or HTTPS path that is the parent of
one or more resources. The server should return a decodable API resource object,
or an api.Status object which contains information about the reason for any
failure.


### client-go

Go clients for talking to a Kubernetes cluster, it uses the RESTClient
underneath to communicate with the API server.

### API server resolver

The API server resolver implements an in memory connection to a fake DNS server.

It works as follows:
- It creates a RESTClient with the provided configuration
    - If the API server hostname provided is an IP address it returns an error,
      since golang doesn't try to resolve IP addresses.
    - It is important to mention that golang doesn't try to resolve via DNS
      domains that are resolvable via /etc/hosts
- It connects to the API server and stores the endpoints IP in its cache
    - The API server endpoints MUST have the same port than the configured API
      server Host in the RESTClient, this is required because the resolver only
      influences the destination IP, not the destination port.
- Any request to resolve the configured API server hostname will return the
  endpoints IPs stored in the cache.
- If the cache is empty it will use the default golang resolver.
- If is not able to reach any of the API servers during a certain time, it will
  fallback to the default golang resolver.

The API server resolver benefits itself from the cache, so if its current
connection breaks, it will try to resolve using the remaining IPs in its cache.

### Example: Resilient client-go using the API server resolver

The API server resolver can resolve the problem of bootstrapping clients, it
also can remove the hard dependency on external Load Balancer to provide High
Availability.

The only thing that it requries is a valid fqdn with an A record to point to a
working API server, directly or indirectly.

Once it connects to the API server, it will download the IP addresses published
by the API servers and will use them for every new request, favoring local
addresses.

Per example, we can configure a cluster with KIND:

KIND automatically installs a Load Balancer in front of the apiservers in a
   multiple control-plane configuration:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: control-plane
- role: control-plane
```

`kind create cluster --config ha.yaml`

We can check the nodes IPs:

```
$ kubectl get nodes -o wide
NAME                  STATUS   ROLES                  AGE   VERSION   INTERNAL-IP   EXTERNAL-IP   OS-IMAGE                                   KERNEL-VERSION                CONTAINER-RUNTIME
kind-control-plane    Ready    control-plane,master   20m   v1.22.1   172.18.0.2    <none>        Ubuntu Impish Indri (development branch)   5.4.132-1.el8.elrepo.x86_64   containerd://1.5.5
kind-control-plane2   Ready    control-plane,master   19m   v1.22.1   172.18.0.4    <none>        Ubuntu Impish Indri (development branch)   5.4.132-1.el8.elrepo.x86_64   containerd://1.5.5
kind-control-plane3   Ready    control-plane,master   18m   v1.22.1   172.18.0.3    <none>        Ubuntu Impish Indri (development branch)   5.4.132-1.el8.elrepo.x86_64   containerd://1.5.5
```

And the Load Balancer IP:
```
$ docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kind-external-load-balancer
172.18.0.2
```

After building the example we can copy it to one of the nodes, kind-control-plane2 per example, since they resolve the Load Balancer hostname.




```
$ cd staging/src/k8s.io/client-go/resolver/example/ && go build main.go
$ docker cp example kind-control-plane2:.                                                                                  
$ docker exec -it kind-control-plane2 ./example -kubeconfig /etc/kubernetes/admin.conf
I0923 16:00:36.580823   11743 resolver.go:229] Starting in memory apiserver resolver ...
I0923 16:00:47.769038   11743 main.go:65] There are 0 pods in the default namespace
I0923 16:00:52.785358   11743 main.go:65] There are 0 pods in the default namespace
I0923 16:00:57.911445   11743 main.go:65] There are 0 pods in the default namespace
I0923 16:01:02.981383   11743 main.go:65] There are 0 pods in the default namespace
...
```


We can see it is connected to the local apiserver, 
```
$ docker exec -it kind-control-plane2 ss -apn | grep example
tcp   ESTAB      0      0                                                                              172.18.0.5:56686               172.18.0.5:6443      users:(("example",pid=12826,fd=6))
```

because 172.18.0.5 is the IP of that node :)
```
$ docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kind-control-plane2
172.18.0.5

```

But it first connected to the Load Balancer:
```
$ docker exec -it kind-control-plane2 grep server /etc/kubernetes/admin.conf
    server: https://kind-external-load-balancer:6443
```
got the API server endpoints:

```
$ docker exec -it kind-control-plane2 kubectl get endpoints kubernetes
NAME         ENDPOINTS                                         AGE
kubernetes   172.18.0.3:6443,172.18.0.4:6443,172.18.0.5:6443   8h
```

closed the idles connections to force the golang dialer to query DNS, and when the golang Dialer asked for `kind-external-load-balancer`,
it returned the IP addresses from the endpoints ordered, using the local one first, since the golang Dialer gives a head start to the first one.

Let's make the local API server unavailable, dropping the traffic with an iptables rule:
```
$ docker exec -it kind-control-plane2 iptables -I INPUT 1 -p tcp --destination-port 6443 -j DROP
```

Kubernetes uses HTTP2 by default and sends a periodic ping frame to detect stale connections:
```
I0923 16:15:42.537605   12826 main.go:65] There are 0 pods in the default namespace
I0923 16:15:47.539683   12826 main.go:65] There are 0 pods in the default namespace
I0923 16:16:32.541253   12826 main.go:62] Error connection: Get "https://kind-external-load-balancer:6443/api/v1/namespaces/default/pods": http2: client connection lost
I0923 16:16:42.551687   12826 main.go:65] There are 0 pods in the default namespace
```

We can see the gap of connectivity is about 45 seconds.

Once the connection is tear down by the inactivity, the client dials again and the resolver sends one of the IPs in the
cache, reconnecting successfully to other one available:

```
docker exec -it kind-control-plane2 ss -apno | grep example
tcp   ESTAB      0      0                                                                              172.18.0.5:36540               172.18.0.4:6443      users:(("example",pid=12826,fd=7)) timer:(keepalive,20sec,0)   
```

