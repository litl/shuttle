shuttle - Dynamic HTTP(S)/TCP/UDP Service Proxy 
=======

![latest v0.1.0](https://img.shields.io/badge/latest-v0.1.0-green.svg?style=flat)
[![Build Status](https://travis-ci.org/litl/shuttle.svg?branch=master)](https://travis-ci.org/litl/shuttle)
![License MIT](https://img.shields.io/badge/license-MIT-blue.svg?style=flat)

Shuttle is a proxy and load balancer, which can be updated live via an HTTP
interface. It can Proxy TCP, UDP, and HTTP(S) via virtual hosts.

## Features
 - TCP/UDP/HTTP/HTTPS (SNI) Proxying
 - Round robin/Least Connection/Weighted Load Balancing
 - Backend Health Checks
 - HTTP API for dynamic updating and querying
 - Stats API
 - HTTP(S) Virtual Host Routing
 - Configuration HTTP Error Pages
 - Optional proxy config state saving
 - Optional file config

## Install

```
$ wget https://github.com/litl/shuttle/releases/download/v0.1.0/shuttle-linux-amd64-v0.1.0.tar.gz
$ tar xvzv shuttle-linux-amd64-v0.1.0.tar.gz
```

## Usage

Shuttle can be started with a default configuration, as well as its last
configuration state. The -state configuration is updated on changes to the
internal config. If the state config file doesn't exist, the default is loaded.
The default config is never written to by shuttle.

Shuttle can serve multiple HTTPS hosts via SNI. Certs are loaded by providing
a directory containing pairs of certificates and keys with the naming
convention, `vhost.name.pem` `vhost.name.key`. 


Basic TCP proxy:

    $ ./shuttle -admin 127.0.0.1:9090 -config default_config.json -state state_config.json


Proxy with a virtualhost HTTP proxy on port 8080:

	$ ./shuttle -admin 127.0.0.1:9090 -http :8080 -config default_config.json -state state_config.json


The current config can be queried via the `/_config` endpoint. This returns a
json list of Services and their Backends, which can be saved directly as a
config file. The configuration itself is defined by `Config` in
github.com/litl/shuttle/client. The running config cam be updated by issuing a
PUT or POST with a valid  json config to `/_config`.

A GET request to `/` or `/_stats` returns the live stats from all Services.
Individual services can be queried by their name, `/service_name`, returning
just the json stats for that service. Backend stats can be queried directly as
well via the path `service_name/backend_name`.

Issuing a PUT with a json config to the service's endpoint will create, or
replace that service. Any changes to the running service require shutting down
the listener, and starting a new service, which will create a very small period
where connection may be rejected.

Issuing a PUT with a json config to the backend's endpoint will create or
replace that backend. Existing connections relying on the old config will
continue to run until the connection is closed.


## TODO

- Documentation!
- Configure individual hosts to require HTTPS
- Connection limits (per service and/or per backend)
- Rate limits
- Mark backend down after non-check connection failures (still requires checks to bring it back up)
- Health check via http, or tcp call/resp pattern
- Protocol bridging? e.g. `TCP<->unix`, `UDP->TCP`?!
- Better logging
- Remove all dependency on galaxy (galaxy/log?)

## License

MIT
