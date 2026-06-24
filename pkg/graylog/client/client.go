package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Verbose  bool
	Endpoint string
	Username string
	Password string
}

type Client struct {
	cfg *Config
}

func NewClient(cfg *Config) *Client {
	client := new(Client)
	client.cfg = cfg
	return client
}

func (client *Client) request(method string, path string, req, resp any) (err error) {

	var body io.Reader
	var reqBody []byte

	if req != nil {
		var buf = new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(req); err != nil {
			return err
		}
		reqBody = buf.Bytes()
		body = bytes.NewBuffer(reqBody)
	}

	url := client.cfg.Endpoint + path

	httpClient := new(http.Client)
	httpReq, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}

	httpReq.SetBasicAuth(client.cfg.Username, client.cfg.Password)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Requested-By", "XMLHttpRequest")
	httpReq.Header.Set("X-Requested-With", "XMLHttpRequest")

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	if client.cfg.Verbose {
		fmt.Println("Request:", method, url, httpResp.Status)
		fmt.Println("Request Body:", strings.TrimSpace(string(reqBody)))
		fmt.Println("Response Body:", strings.TrimSpace(string(respBody)))
	}

	if httpResp.StatusCode > 299 {
		return errors.Errorf("%v / %v", httpResp.Status, strings.TrimSpace(string(respBody)))
	}

	buff := bytes.NewBuffer(respBody)
	if err := json.NewDecoder(buff).Decode(&resp); err != nil {
		return err
	}

	return nil
}

func (client *Client) Post(path string, req, resp any) (err error) {
	return client.request("POST", path, req, resp)
}

func (client *Client) Get(path string, req, resp any) (err error) {
	return client.request("GET", path, req, resp)
}
