package pcfas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

type Client struct {
	client *http.Client
	host   string
	token  string
	trace  io.Writer
}

func NewClient(host, token string) *Client {
	return &Client{
		host:   host,
		token:  token,
		client: &http.Client{},
	}
}

func (p *Client) TraceTo(writer io.Writer) {
	p.trace = writer
}

type Pagination struct {
	TotalPages int `json:"total_pages"`
}

func (p *Client) newRequest(method, path string, query map[string]string, body interface{}) (*http.Request, error) {
	values := url.Values{}
	for k, v := range query {
		values.Set(k, v)
	}

	var bodyReader io.ReadWriter
	if body != nil {
		body = bytes.Buffer{}
		jEncoder := json.NewEncoder(bodyReader)
		err := jEncoder.Encode(body)
		if err != nil {
			return nil, err
		}
	}

	u := url.URL{
		Scheme:   "https",
		Host:     p.host,
		Path:     path,
		RawQuery: values.Encode(),
	}

	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/json")

	return req, nil
}

type AppsResponse struct {
	Pagination Pagination `json:"pagination"`
	Resources  []App      `json:"resources"`
}

func (p *Client) doRequest(request *http.Request, out interface{}) error {
	if p.trace != nil {
		reqDump, err := httputil.DumpRequestOut(request, true)
		if err != nil {
			return fmt.Errorf("Error dumping request: %s", err)
		}

		_, err = p.trace.Write(reqDump)
		if err != nil {
			return fmt.Errorf("Error writing request dump: %s", err)
		}
	}

	resp, err := p.client.Do(request)
	if err != nil {
		return err
	}

	defer func() {
		_, _ = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}()

	if p.trace != nil {
		respDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return fmt.Errorf("Error dumping response: %s", err)
		}

		_, err = p.trace.Write(respDump)
		if err != nil {
			return fmt.Errorf("Error writing response dump: %s", err)
		}
	}

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Non-2xx response code: %s", resp.Status)
	}

	if out == nil {
		return nil
	}

	jDecoder := json.NewDecoder(resp.Body)
	err = jDecoder.Decode(&out)
	if err != nil {
		return err
	}

	return err
}

type App struct {
	Enabled        bool           `json:"enabled"`
	GUID           string         `json:"guid"`
	InstanceLimits InstanceLimits `json:"instance_limits"`
}

type InstanceLimits struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func (p *Client) AppsForSpaceWithGUID(guid string) ([]App, error) {
	currentPage := 1
	resources := []App{}
	for {
		responseBodyObj := AppsResponse{}
		req, err := p.newRequest(
			"GET",
			"/api/v2/apps",
			map[string]string{
				"space_guid": guid,
				"page":       strconv.Itoa(currentPage),
			},
			nil,
		)
		if err != nil {
			return nil, err
		}

		err = p.doRequest(req, &responseBodyObj)
		if err != nil {
			return nil, err
		}

		resources = append(resources, responseBodyObj.Resources...)

		totalPages := responseBodyObj.Pagination.TotalPages
		if totalPages >= currentPage {
			break
		}

		currentPage++
	}

	return resources, nil
}
