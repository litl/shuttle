package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// Client is an http client for communicating with the shuttle server api
type Client struct {
	httpClient *http.Client
	addr       string
}

// An http client for communicating with the shuttle server.
func NewClient(addr string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 2 * time.Second},
		addr:       addr,
	}
}

// GetConfig retrieves the configuration for a running shuttle server.
func (c *Client) GetConfig() (*Config, error) {

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/_config", c.addr), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = json.Unmarshal(body, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// UpdateConfig updates the running config on a shuttle server. This will
// update globals settings and add services, but currently doesn't remove any
// running service or backends.
func (c *Client) UpdateConfig(config *Config) error {

	js, err := json.Marshal(config)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(fmt.Sprintf("http://%s/_config", c.addr), "application/json",
		bytes.NewBuffer(js))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update shuttle config: %s", resp.Status)
	}
	return nil
}

// UpdateService adds or updates a service on a running shuttle server.
func (c *Client) UpdateService(service *ServiceConfig) error {

	js, err := json.Marshal(service)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(fmt.Sprintf("http://%s/%s", c.addr, service.Name), "application/json",
		bytes.NewBuffer(js))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update shuttle service '%s': %s", service.Name, resp.Status)
	}
	return nil
}

// RemoveService removes a service and its backends from a running shuttle server.
func (c *Client) RemoveService(service string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://%s/%s", c.addr, service), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("failed to remove shuttle service '%s': %s", service, resp.Status))
	}
	return nil
}

// UpdateBackend adds or updates a single backend on a running shuttle server.
func (c *Client) UpdateBackend(service string, backend *BackendConfig) error {

	js, err := json.Marshal(backend)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(fmt.Sprintf("http://%s/%s/%s", c.addr, service, backend.Name), "application/json",
		bytes.NewBuffer(js))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update shuttle backend '%s/%s': %s", service, backend.Name, resp.Status)
	}
	return nil
}

// RemoveBackend removes a backend from its service on a running shuttle server.
func (c *Client) RemoveBackend(service, backend string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://%s/%s/%s", c.addr, service, backend), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("failed to remove shuttle backend '%s/%s': %s", service, backend, resp.Status))
	}
	return nil
}
