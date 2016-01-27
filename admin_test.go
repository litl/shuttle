package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"

	"github.com/litl/shuttle/client"
	. "gopkg.in/check.v1"
)

type HTTPSuite struct {
	servers        []*testServer
	backendServers []*testHTTPServer
	httpSvr        *httptest.Server
	httpAddr       string
	httpPort       string
	httpsAddr      string
	httpsPort      string
}

var _ = Suite(&HTTPSuite{})

var (
	tempPort = 19000
	// protects tempPort
	portMu sync.Mutex
)

// return a unique 127.0.0.1:PORT for each test
func localPort() string {
	portMu.Lock()
	defer portMu.Unlock()
	tempPort++
	return fmt.Sprintf("127.0.0.1:%d", tempPort)
}

func localAddrBody() *bytes.Reader {
	return bytes.NewReader([]byte(fmt.Sprintf(`{"address": "%s"}`, localPort())))
}

func (s *HTTPSuite) SetUpSuite(c *C) {
	Registry = ServiceRegistry{
		svcs:   make(map[string]*Service),
		vhosts: make(map[string]*VirtualHost),
	}

	addHandlers()
	s.httpSvr = httptest.NewServer(nil)

	httpServer := &http.Server{
		Addr: "127.0.0.1:0",
	}

	httpRouter = NewHostRouter(httpServer)
	httpReady := make(chan bool)
	go httpRouter.Start(httpReady)
	<-httpReady

	// now build an HTTPS server
	tlsCfg, err := loadCerts("./testdata")
	if err != nil {
		c.Fatal(err)
		return
	}

	httpsServer := &http.Server{
		Addr:      "127.0.0.1:0",
		TLSConfig: tlsCfg,
	}

	httpsRouter := NewHostRouter(httpsServer)
	httpsRouter.Scheme = "https"

	httpsReady := make(chan bool)
	go httpsRouter.Start(httpsReady)
	<-httpsReady

	// assign the test router's addr to the glolbal
	s.httpAddr = httpRouter.listener.Addr().String()
	s.httpPort = fmt.Sprintf("%d", httpRouter.listener.Addr().(*net.TCPAddr).Port)
	s.httpsAddr = httpsRouter.listener.Addr().String()
	s.httpsPort = fmt.Sprintf("%d", httpsRouter.listener.Addr().(*net.TCPAddr).Port)
}

func (s *HTTPSuite) TearDownSuite(c *C) {
	s.httpSvr.Close()
	httpRouter.Stop()
}

func (s *HTTPSuite) SetUpTest(c *C) {
	// start 4 possible backend servers
	for i := 0; i < 4; i++ {
		server, err := NewTestServer("127.0.0.1:0", c)
		if err != nil {
			c.Fatal(err)
		}
		s.servers = append(s.servers, server)
	}

	for i := 0; i < 4; i++ {
		server, err := NewHTTPTestServer("127.0.0.1:0", c)
		if err != nil {
			c.Fatal(err)
		}

		s.backendServers = append(s.backendServers, server)
	}
}

// shutdown our backend servers
func (s *HTTPSuite) TearDownTest(c *C) {
	for _, s := range s.servers {
		s.Stop()
	}

	s.servers = s.servers[:0]

	// clear global defaults in Registry
	Registry.cfg.Balance = ""
	Registry.cfg.CheckInterval = 0
	Registry.cfg.Fall = 0
	Registry.cfg.Rise = 0
	Registry.cfg.ClientTimeout = 0
	Registry.cfg.ServerTimeout = 0
	Registry.cfg.DialTimeout = 0

	for _, s := range s.backendServers {
		s.Close()
	}

	s.backendServers = s.backendServers[:0]

	for _, svc := range Registry.svcs {
		Registry.RemoveService(svc.Name)
	}

}

// These don't yet *really* test anything other than code coverage
func (s *HTTPSuite) TestAddService(c *C) {
	svcDef := localAddrBody()
	req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/testService", svcDef)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	c.Assert(Registry.String(), DeepEquals, string(body))
}

func (s *HTTPSuite) TestAddBackend(c *C) {
	svcDef := localAddrBody()
	req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/testService", svcDef)
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}

	backendDef := localAddrBody()
	req, _ = http.NewRequest("PUT", s.httpSvr.URL+"/testService/testBackend", backendDef)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	c.Assert(Registry.String(), DeepEquals, string(body))
}

func (s *HTTPSuite) TestReAddBackend(c *C) {
	svcDef := localAddrBody()
	req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/testService", svcDef)
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}

	backendDef := localAddrBody()
	req, _ = http.NewRequest("PUT", s.httpSvr.URL+"/testService/testBackend", backendDef)
	firstResp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	defer firstResp.Body.Close()

	firstBody, _ := ioutil.ReadAll(firstResp.Body)

	backendDef.Seek(0, 0)
	req, _ = http.NewRequest("PUT", s.httpSvr.URL+"/testService/testBackend", backendDef)
	secResp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	defer secResp.Body.Close()

	secBody, _ := ioutil.ReadAll(secResp.Body)

	c.Assert(string(secBody), DeepEquals, string(firstBody))
}

func (s *HTTPSuite) TestSimulAdd(c *C) {
	start := make(chan struct{})
	testWG := new(sync.WaitGroup)

	svcCfg := client.ServiceConfig{
		Name:         "TestService",
		Addr:         localPort(),
		VirtualHosts: []string{"test-vhost"},
		Backends: []client.BackendConfig{
			client.BackendConfig{
				Name: "vhost1",
				Addr: localPort(),
			},
			client.BackendConfig{
				Name: "vhost2",
				Addr: localPort(),
			},
		},
	}

	for i := 0; i < 8; i++ {
		testWG.Add(1)
		go func() {
			defer testWG.Done()
			//wait to start all at once
			<-start
			svcDef := bytes.NewReader(svcCfg.Marshal())
			req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/TestService", svcDef)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				c.Fatal(err)
			}

			body, _ := ioutil.ReadAll(resp.Body)

			respCfg := client.Config{}
			err = json.Unmarshal(body, &respCfg)

			// We're only checking to ensure we have 1 service with the proper number of backends
			c.Assert(len(respCfg.Services), Equals, 1)
			c.Assert(len(respCfg.Services[0].Backends), Equals, 2)
			c.Assert(len(respCfg.Services[0].VirtualHosts), Equals, 1)
		}()
	}

	close(start)
	testWG.Wait()
}

func (s *HTTPSuite) TestRouter(c *C) {
	svcCfg := client.ServiceConfig{
		Name:         "VHostTest",
		Addr:         localPort(),
		VirtualHosts: []string{"test-vhost"},
	}

	for _, srv := range s.backendServers {
		cfg := client.BackendConfig{
			Addr: srv.addr,
			Name: srv.addr,
		}
		svcCfg.Backends = append(svcCfg.Backends, cfg)
	}

	err := Registry.AddService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	for _, srv := range s.backendServers {
		checkHTTP("http://"+s.httpAddr+"/addr", "test-vhost", srv.addr, 200, c)
	}
}

func (s *HTTPSuite) TestAddRemoveVHosts(c *C) {
	svcCfg := client.ServiceConfig{
		Name:         "VHostTest",
		Addr:         localPort(),
		VirtualHosts: []string{"test-vhost"},
	}

	for _, srv := range s.backendServers {
		cfg := client.BackendConfig{
			Addr: srv.addr,
			Name: srv.addr,
		}
		svcCfg.Backends = append(svcCfg.Backends, cfg)
	}

	err := Registry.AddService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	// now update the service with another vhost
	svcCfg.VirtualHosts = append(svcCfg.VirtualHosts, "test-vhost-2")
	err = Registry.UpdateService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	if Registry.VHostsLen() != 2 {
		c.Fatal("missing new vhost")
	}

	// remove the first vhost
	svcCfg.VirtualHosts = []string{"test-vhost-2"}
	err = Registry.UpdateService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	if Registry.VHostsLen() != 1 {
		c.Fatal("extra vhost:", Registry.VHostsLen())
	}

	// check responses from this new vhost
	for _, srv := range s.backendServers {
		checkHTTP("http://"+s.httpAddr+"/addr", "test-vhost-2", srv.addr, 200, c)
	}
}

// Add multiple services under the same VirtualHost
// Each proxy request should round-robin through the two of them
func (s *HTTPSuite) TestMultiServiceVHost(c *C) {
	svcCfgOne := client.ServiceConfig{
		Name:         "VHostTest",
		Addr:         localPort(),
		VirtualHosts: []string{"test-vhost"},
	}

	svcCfgTwo := client.ServiceConfig{
		Name:         "VHostTest2",
		Addr:         localPort(),
		VirtualHosts: []string{"test-vhost-2"},
	}

	var backends []client.BackendConfig
	for _, srv := range s.backendServers {
		cfg := client.BackendConfig{
			Addr: srv.addr,
			Name: srv.addr,
		}
		backends = append(backends, cfg)
	}

	svcCfgOne.Backends = backends
	svcCfgTwo.Backends = backends

	err := Registry.AddService(svcCfgOne)
	if err != nil {
		c.Fatal(err)
	}

	err = Registry.AddService(svcCfgTwo)
	if err != nil {
		c.Fatal(err)
	}

	for _, srv := range s.backendServers {
		checkHTTP("http://"+s.httpAddr+"/addr", "test-vhost", srv.addr, 200, c)
		checkHTTP("http://"+s.httpAddr+"/addr", "test-vhost-2", srv.addr, 200, c)
	}
}

func (s *HTTPSuite) TestAddRemoveBackends(c *C) {
	svcCfg := client.ServiceConfig{
		Name: "VHostTest",
		Addr: localPort(),
	}

	err := Registry.AddService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	for _, srv := range s.backendServers {
		cfg := client.BackendConfig{
			Addr: srv.addr,
			Name: srv.addr,
		}
		svcCfg.Backends = append(svcCfg.Backends, cfg)
	}

	err = Registry.UpdateService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	cfg := Registry.Config()
	if !svcCfg.DeepEqual(cfg.Services[0]) {
		c.Errorf("we should have 1 service, we have %d", len(cfg.Services))
		c.Errorf("we should have 4 backends, we have %d", len(cfg.Services[0].Backends))
	}

	svcCfg.Backends = svcCfg.Backends[:3]
	err = Registry.UpdateService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	cfg = Registry.Config()
	if !svcCfg.DeepEqual(cfg.Services[0]) {
		c.Errorf("we should have 1 service, we have %d", len(cfg.Services))
		c.Errorf("we should have 3 backends, we have %d", len(cfg.Services[0].Backends))
	}

}

func (s *HTTPSuite) TestHTTPAddRemoveBackends(c *C) {
	svcCfg := client.ServiceConfig{
		Name: "VHostTest",
		Addr: localPort(),
	}

	err := Registry.AddService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	for _, srv := range s.backendServers {
		cfg := client.BackendConfig{
			Addr: srv.addr,
			Name: srv.addr,
		}
		svcCfg.Backends = append(svcCfg.Backends, cfg)
	}

	req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/VHostTest", bytes.NewReader(svcCfg.Marshal()))
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}

	cfg := Registry.Config()
	if !svcCfg.DeepEqual(cfg.Services[0]) {
		c.Errorf("we should have 1 service, we have %d", len(cfg.Services))
		c.Errorf("we should have 4 backends, we have %d", len(cfg.Services[0].Backends))
	}

	// remove a backend from the config and submit it again
	svcCfg.Backends = svcCfg.Backends[:3]
	err = Registry.UpdateService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	req, _ = http.NewRequest("PUT", s.httpSvr.URL+"/VHostTest", bytes.NewReader(svcCfg.Marshal()))
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}

	// now check the config via what's returned from the http server
	resp, err := http.Get(s.httpSvr.URL + "/_config")
	if err != nil {
		c.Fatal(err)
	}
	defer resp.Body.Close()

	cfg = client.Config{}
	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &cfg)
	if err != nil {
		c.Fatal(err)
	}

	if !svcCfg.DeepEqual(cfg.Services[0]) {
		c.Errorf("we should have 1 service, we have %d", len(cfg.Services))
		c.Errorf("we should have 3 backends, we have %d", len(cfg.Services[0].Backends))
	}
}

func (s *HTTPSuite) TestErrorPage(c *C) {
	svcCfg := client.ServiceConfig{
		Name:         "VHostTest",
		Addr:         localPort(),
		VirtualHosts: []string{"test-vhost"},
	}

	okServer := s.backendServers[0]
	errServer := s.backendServers[1]

	// Add one backend to service requests
	cfg := client.BackendConfig{
		Addr: okServer.addr,
		Name: okServer.addr,
	}
	svcCfg.Backends = append(svcCfg.Backends, cfg)

	// use another backend to provide the error page
	svcCfg.ErrorPages = map[string][]int{
		"http://" + errServer.addr + "/error": []int{400, 503},
	}

	err := Registry.AddService(svcCfg)
	if err != nil {
		c.Fatal(err)
	}

	// check that the normal response comes from srv1
	checkHTTP("http://"+s.httpAddr+"/addr", "test-vhost", okServer.addr, 200, c)
	// verify that an unregistered error doesn't give the cached page
	checkHTTP("http://"+s.httpAddr+"/error?code=504", "test-vhost", okServer.addr, 504, c)
	// now see if the registered error comes from srv2
	checkHTTP("http://"+s.httpAddr+"/error?code=503", "test-vhost", errServer.addr, 503, c)

	// now check that we got the header cached in the error page as well
	req, err := http.NewRequest("GET", "http://"+s.httpAddr+"/error?code=503", nil)
	if err != nil {
		c.Fatal(err)
	}

	req.Host = "test-vhost"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}

	c.Assert(resp.StatusCode, Equals, 503)
	c.Assert(resp.Header.Get("Last-Modified"), Equals, errServer.addr)
}

func (s *HTTPSuite) TestUpdateServiceDefaults(c *C) {
	svcCfg := client.ServiceConfig{
		Name: "TestService",
		Addr: localPort(),
		Backends: []client.BackendConfig{
			client.BackendConfig{
				Name: "Backend1",
				Addr: localPort(),
			},
		},
	}

	svcDef := bytes.NewBuffer(svcCfg.Marshal())
	req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/TestService", svcDef)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	resp.Body.Close()

	// Now update the Service in-place
	svcCfg.ServerTimeout = 1234
	svcDef.Reset()
	svcDef.Write(svcCfg.Marshal())

	req, _ = http.NewRequest("PUT", s.httpSvr.URL+"/TestService", svcDef)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	config := client.Config{}
	err = json.Unmarshal(body, &config)
	if err != nil {
		c.Fatal(err)
	}

	// make sure we don't see a second value
	found := false

	for _, svc := range config.Services {
		if svc.Name == "TestService" {
			if svc.ServerTimeout != svcCfg.ServerTimeout {
				c.Fatal("Service not updated")
			} else if found {
				c.Fatal("Multiple Service Definitions")
			}
			found = true
		}
	}
}

// Set some global defaults, and check that a new service inherits them all
func (s *HTTPSuite) TestGlobalDefaults(c *C) {
	globalCfg := client.Config{
		Balance:       "LC",
		CheckInterval: 101,
		Fall:          7,
		Rise:          8,
		ClientTimeout: 102,
		ServerTimeout: 103,
		DialTimeout:   104,
	}

	globalDef := bytes.NewBuffer(globalCfg.Marshal())
	req, _ := http.NewRequest("PUT", s.httpSvr.URL+"/", globalDef)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	resp.Body.Close()

	svcCfg := client.ServiceConfig{
		Name: "TestService",
		Addr: localPort(),
	}

	svcDef := bytes.NewBuffer(svcCfg.Marshal())
	req, _ = http.NewRequest("PUT", s.httpSvr.URL+"/TestService", svcDef)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		c.Fatal(err)
	}
	resp.Body.Close()

	config := Registry.Config()

	c.Assert(len(config.Services), Equals, 1)
	service := config.Services[0]

	c.Assert(globalCfg.Balance, Equals, service.Balance)
	c.Assert(globalCfg.CheckInterval, Equals, service.CheckInterval)
	c.Assert(globalCfg.Fall, Equals, service.Fall)
	c.Assert(globalCfg.Rise, Equals, service.Rise)
	c.Assert(globalCfg.ClientTimeout, Equals, service.ClientTimeout)
	c.Assert(globalCfg.ServerTimeout, Equals, service.ServerTimeout)
	c.Assert(globalCfg.DialTimeout, Equals, service.DialTimeout)
}

// Test that we can route to Vhosts based on SNI
func (s *HTTPSuite) TestHTTPSRouter(c *C) {
	srv1 := s.backendServers[0]
	srv2 := s.backendServers[1]

	svcCfgOne := client.ServiceConfig{
		Name:         "VHostTest1",
		Addr:         localPort(),
		VirtualHosts: []string{"vhost1.test", "alt.vhost1.test", "star.vhost1.test"},
		Backends: []client.BackendConfig{
			{Addr: srv1.addr},
		},
	}

	svcCfgTwo := client.ServiceConfig{
		Name:         "VHostTest2",
		Addr:         localPort(),
		VirtualHosts: []string{"vhost2.test", "alt.vhost2.test", "star.vhost2.test"},
		Backends: []client.BackendConfig{
			{Addr: srv2.addr},
		},
	}

	err := Registry.AddService(svcCfgOne)
	if err != nil {
		c.Fatal(err)
	}

	err = Registry.AddService(svcCfgTwo)
	if err != nil {
		c.Fatal(err)
	}

	// Our router has 2 certs, each with name.test and alt.name.test as DNS names.
	// checkHTTP has a fake dialer that resolves everything to 127.0.0.1.
	checkHTTP("https://vhost1.test:"+s.httpsPort+"/addr", "vhost1.test", srv1.addr, 200, c)
	checkHTTP("https://alt.vhost1.test:"+s.httpsPort+"/addr", "alt.vhost1.test", srv1.addr, 200, c)
	checkHTTP("https://star.vhost1.test:"+s.httpsPort+"/addr", "star.vhost1.test", srv1.addr, 200, c)

	checkHTTP("https://vhost2.test:"+s.httpsPort+"/addr", "vhost2.test", srv2.addr, 200, c)
	checkHTTP("https://alt.vhost2.test:"+s.httpsPort+"/addr", "alt.vhost2.test", srv2.addr, 200, c)
	checkHTTP("https://star.vhost2.test:"+s.httpsPort+"/addr", "star.vhost2.test", srv2.addr, 200, c)
}

// Verify that Settting HTTPSRedirect on a service works as expected for https
// and for X-Forwarded-Proto:https.
func (s *HTTPSuite) TestHTTPSRedirect(c *C) {
	srv1 := s.backendServers[0]

	svcCfgOne := client.ServiceConfig{
		Name:          "VHostTest1",
		Addr:          localPort(),
		HTTPSRedirect: true,
		VirtualHosts:  []string{"vhost1.test", "alt.vhost1.test", "star.vhost1.test"},
		Backends: []client.BackendConfig{
			{Addr: srv1.addr},
		},
	}

	err := Registry.AddService(svcCfgOne)
	if err != nil {
		c.Fatal(err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: localDial,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("redirected")
		},
	}

	// this should redirect to https
	reqHTTP, _ := http.NewRequest("HEAD", "http://localhost:"+s.httpPort+"/addr", nil)
	reqHTTP.Host = "vhost1.test"

	resp, err := client.Do(reqHTTP)
	if err != nil {
		if err, ok := err.(*url.Error); ok {
			if err.Err.Error() != "redirected" {
				c.Fatal(err)
			}
		} else {
			c.Fatal(err)
		}
	}
	c.Assert(resp.StatusCode, Equals, http.StatusMovedPermanently)

	// this should be OK
	reqHTTP.Header = map[string][]string{
		"X-Forwarded-Proto": {"https"},
	}
	resp, err = client.Do(reqHTTP)
	if err != nil {
		c.Fatal(err)
	}
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
}

func (s *HTTPSuite) TestMaintenanceMode(c *C) {
	mainServer := s.backendServers[0]
	errServer := s.backendServers[1]

	svcCfg := client.ServiceConfig{
		Name:         "VHostTest1",
		Addr:         localPort(),
		VirtualHosts: []string{"vhost1.test"},
		Backends: []client.BackendConfig{
			{Addr: mainServer.addr},
		},
		MaintenanceMode: true,
	}

	if err := Registry.AddService(svcCfg); err != nil {
		c.Fatal(err)
	}

	// No error page is registered, so we should just get a 503 error with no body
	checkHTTP("https://vhost1.test:"+s.httpsPort+"/addr", "vhost1.test", "", 503, c)

	// Use another backend to provide the error page
	svcCfg.ErrorPages = map[string][]int{
		"http://" + errServer.addr + "/error?code=503": []int{503},
	}

	if err := Registry.UpdateService(svcCfg); err != nil {
		c.Fatal(err)
	}

	// Get a 503 error with the cached body
	checkHTTP("https://vhost1.test:"+s.httpsPort+"/addr", "vhost1.test", errServer.addr, 503, c)

	// Turn maintenance mode off
	svcCfg.MaintenanceMode = false

	if err := Registry.UpdateService(svcCfg); err != nil {
		c.Fatal(err)
	}

	checkHTTP("https://vhost1.test:"+s.httpsPort+"/addr", "vhost1.test", mainServer.addr, 200, c)

	// Turn it back on
	svcCfg.MaintenanceMode = true

	if err := Registry.UpdateService(svcCfg); err != nil {
		c.Fatal(err)
	}

	checkHTTP("https://vhost1.test:"+s.httpsPort+"/addr", "vhost1.test", errServer.addr, 503, c)
}
