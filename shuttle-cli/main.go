package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	shuttle "github.com/litl/shuttle/client"
)

var (
	shuttleAddr string
	configData  string
	configFile  string

	buildVersion = "0.1.0"

	client *shuttle.Client

	cfg      = &shuttle.Config{}
	configFS = flag.NewFlagSet("config", flag.ExitOnError)

	serviceCfg = &shuttle.ServiceConfig{}
	serviceFS  = flag.NewFlagSet("service", flag.ExitOnError)
	vhosts     = stringSlice{}
	errorPages = stringSlice{}

	backendCfg = &shuttle.BackendConfig{}
	backendFS  = flag.NewFlagSet("backend", flag.ExitOnError)
)

func init() {
	configFS.StringVar(&cfg.Balance, "balance", "", "balance algorithm, {RR|LC}")
	configFS.IntVar(&cfg.CheckInterval, "check-interval", 0, "interval between health checks in milliseconds")
	configFS.IntVar(&cfg.Fall, "fall", 0, "number of failed healthchecks before a backend is marked down")
	configFS.IntVar(&cfg.Rise, "rise", 0, "number of successful health checks before a down service is marked up")
	configFS.IntVar(&cfg.ClientTimeout, "client-timeout", 0, "innactivity timeout for client connections")
	configFS.IntVar(&cfg.ServerTimeout, "server-timeout", 0, "innactivity timeout for server connections")
	configFS.IntVar(&cfg.DialTimeout, "dial-timeout", 0, "timeout for dialing new connections connections")

	serviceFS.StringVar(&serviceCfg.Addr, "address", "", "service listening address")
	serviceFS.StringVar(&serviceCfg.Network, "network", "", "service network type")
	serviceFS.StringVar(&serviceCfg.Balance, "balance", "", "balancing algorithm, {RR|LC}")
	serviceFS.IntVar(&serviceCfg.CheckInterval, "check-interval", 0, "interval between health checks in milliseconds")
	serviceFS.IntVar(&serviceCfg.Fall, "fall", 0, "number of failed healthchecks before a backend is marked down")
	serviceFS.IntVar(&serviceCfg.Rise, "rise", 0, "number of successful health checks before a down service is marked up")
	serviceFS.IntVar(&serviceCfg.ClientTimeout, "client-timeout", 0, "innactivity timeout for client connections")
	serviceFS.IntVar(&serviceCfg.ServerTimeout, "server-timeout", 0, "innactivity timeout for server connections")
	serviceFS.IntVar(&serviceCfg.DialTimeout, "dial-timeout", 0, "timeout for dialing new connections connections")
	serviceFS.Var(&vhosts, "vhost", "virtual host name. may be set multiple times")
	serviceFS.Var(&errorPages, "error-page", "location for http error code formatted as 'http://example.com/|500,503'. may be set multiple times")

	backendFS.StringVar(&backendCfg.Addr, "address", "", "service listening address")
	backendFS.StringVar(&backendCfg.Network, "network", "", "backend network type")
	backendFS.StringVar(&backendCfg.CheckAddr, "check-address", "", "health check address")
	backendFS.IntVar(&backendCfg.Weight, "weight", 0, "balance weight")
}

func usage() {
	flag.PrintDefaults()
	fmt.Println(`shuttle-cli {config|update|remove} [options]

config [options]
         set or print global config
example: update the default client-timeout to 10 seconds
         $ shuttle-cli config -client-timeout 10s
options:`)
	configFS.PrintDefaults()

	fmt.Println(`
update service [options]
         add or update a service
example: update the server-timeout on "servicename" to 2 seconds
         $ shuttle-cli update servicename -server-timeout 2s
options:`)
	serviceFS.PrintDefaults()

	fmt.Println(`
udpate service/backend [options]
         add or update a backend
example: update the round-robin weight on "service/backend" to 3
         $ shuttle-cli update service/backend -weight 3
options:`)
	backendFS.PrintDefaults()

	fmt.Println(`
remove: remove services or backends
        remove service
        remove service/backend`)

	os.Exit(1)
}

func main() {
	log.SetPrefix("")
	log.SetFlags(0)

	flag.StringVar(&shuttleAddr, "addr", "127.0.0.1:9090", "shuttle admin address")
	flag.Usage = usage

	flag.Parse()

	if flag.NArg() < 1 {
		usage()

	}

	client = shuttle.NewClient(shuttleAddr)

	switch flag.Args()[0] {
	case "version":
		fmt.Println(buildVersion)
		return
	case "config":
		config(flag.Args()[1:])
	case "update", "add":
		update(flag.Args()[1:])
	case "remove":
		remove(flag.Args()[1:])
	default:
		usage()
	}

}

func config(args []string) {
	if len(args) == 0 {
		cfg, err := client.GetConfig()
		if err != nil {
			log.Fatal(err)
		}

		js, err := json.MarshalIndent(cfg, " ", "")
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(js))
		return
	}

	configFS.Parse(args)

	err := client.UpdateConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
}

// slice for multiple string flags
type stringSlice []string

func (s *stringSlice) Set(arg string) error {
	*s = append(*s, arg)
	return nil
}

func (s stringSlice) String() string {
	return strings.Join(s, ",")
}

func update(args []string) {
	if len(args) < 1 {
		usage()
	}

	target := strings.SplitN(args[0], "/", 2)
	if len(target) == 1 {
		updateService(target[0], args[1:])
		return
	}

	updateBackend(target[0], target[1], args[1:])
}

func updateService(service string, args []string) {
	serviceFS.Parse(args)

	if len(vhosts) > 0 {
		serviceCfg.VirtualHosts = vhosts
	}

	if len(errorPages) > 1 {
		serviceCfg.ErrorPages = parseErrorPages(errorPages)
	}

	serviceCfg.Name = service

	err := client.UpdateService(serviceCfg)
	if err != nil {
		log.Fatal(err)
	}
}

func parseErrorPages(pages []string) map[string][]int {
	ep := make(map[string][]int)
	for _, p := range pages {
		parts := strings.SplitN(p, "|", 1)
		if len(parts) != 2 {
			log.Fatalf("invalid error-page %s", p)
		}

		codes := []int{}
		for _, code := range strings.Split(parts[1], ",") {
			i, err := strconv.Atoi(code)
			if err != nil {
				log.Fatalf("invalided error-page %s, %s", p, err.Error())
			}
			codes = append(codes, i)
		}

		ep[p] = codes
	}
	return ep
}

func updateBackend(service, backend string, args []string) {
	backendFS.Parse(args)

	backendCfg.Name = backend
	err := client.UpdateBackend(service, backendCfg)
	if err != nil {
		log.Fatal(err)
	}
}

func remove(args []string) {
	if len(args) < 1 {
		usage()
	}

	target := strings.SplitN(args[0], "/", 1)
	if len(target) == 1 {
		err := client.RemoveService(target[0])
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	err := client.RemoveBackend(target[0], target[1])
	if err != nil {
		log.Fatal(err)
	}

}
