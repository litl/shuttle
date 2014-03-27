shuttle TCP Proxy
=======

Shuttle is a TCP proxy and load balancer, which can be updated live via an HTTP
interface.

## Usage
Shuttle can be started with a default configuration, as well as its last
configuration state. The state configuration is updated on changes to the
internal config. If the state config file doesn't exist, the default is loaded.

    $ ./shuttle -config default_config.json -state state_config.json


The current config can be queried via the `/config` endpoint. This returns a
json list of Services and their Backends, which can be saved directly as a
config file. The configuration itself is a list of Services, which each make
have a list of Backends. 

A GET request to the path `/` returns an extended json config containing live
stats from all Services. Individual services can be queried by their name,
`/service_name`, returning just the json stats for that service. Issuing a PUT
with a json config to the service's endpoint will create, or update that
service. Backends can be included in the service's json, and configured
separately.

A GET request to a backend at `/service_name/backend_name` will return the
current stats for just that backend. Issuing a PUT with a json config to the
backend's endpoint will create or update that backend.
