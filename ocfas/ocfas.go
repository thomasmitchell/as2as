package ocfas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type Policy struct {
	InstanceMinCount int64         `json:"instance_min_count"`
	InstanceMaxCount int64         `json:"instance_max_count"`
	ScalingRules     []ScalingRule `json:"scaling_rules,omitempty"`
	Schedules        Schedules     `json:"schedules,omitempty"`
}

const (
	MetricTypeMemoryUsed   = "memoryused"
	MetricTypeMemoryUtil   = "memoryutil"
	MetricTypeCPUUtil      = "cpu"
	MetricTypeResponseTime = "responsetime"
	MetricTypeThroughput   = "throughput"
	MetricTypeCustom       = "custom"
)

const (
	OperatorLessThan             string = "<"
	OperatorLessThanOrEqualTo    string = "<="
	OperatorGreaterThan          string = ">"
	OperatorGreaterThanOrEqualTo string = ">="
)

const (
	AdjustmentDown string = "-1"
	AdjustmentUp   string = "+1"
)

type ScalingRule struct {
	MetricType         string `json:"metric_type"`
	Operator           string `json:"operator"`
	Threshold          int64  `json:"threshold"`
	Adjustment         string `json:"adjustment"`
	CooldownSecs       int64  `json:"cool_down_secs,omitempty"`
	BreachDurationSecs int64  `json:"breach_duration_secs,omitempty"`
}

const (
	TimezoneUTC = "Etc/UTC"
)

type Schedules struct {
	Timezone          string              `json:"timezone,omitempty"`
	RecurringSchedule []RecurringSchedule `json:"recurring_schedule,omitempty"`
	SpecificDate      []SpecificDate      `json:"specific_date,omitempty"`
}

type RecurringSchedule struct {
	StartTime        string     `json:"start_time"`
	EndTime          string     `json:"end_time"`
	DaysOfWeek       DaysOfWeek `json:"days_of_week"`
	InstanceMinCount int64      `json:"instance_min_count"`
	InstanceMaxCount int64      `json:"instance_max_count"`
	//TODO: Make into pointer?
	InitialMinInstanceCount *int64 `json:"initial_min_instance_count,omitempty"`
}

type DaysOfWeek []int8

type SpecificDate struct {
	StartDateTime           string `json:"start_date_time"`
	EndDateTime             string `json:"end_date_time"`
	InstanceMinCount        int64  `json:"instance_min_count"`
	InstanceMaxCount        int64  `json:"instance_max_count"`
	InitialMinInstanceCount *int64 `json:"initial_min_instance_count,omitempty"`
}

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

func (c *Client) TraceTo(writer io.Writer) {
	c.trace = writer
}

func (c *Client) newRequest(method, path string, query map[string]string, body interface{}) (*http.Request, error) {
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
		Host:     c.host,
		Path:     path,
		RawQuery: values.Encode(),
	}

	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func (c *Client) doRequest(request *http.Request, out interface{}) error {
	if c.trace != nil {
		reqDump, err := httputil.DumpRequestOut(request, true)
		if err != nil {
			return fmt.Errorf("Error dumping request: %s", err)
		}

		_, err = c.trace.Write(append(reqDump, []byte("\n  ***\n\n")...))
		if err != nil {
			return fmt.Errorf("Error writing request dump: %s", err)
		}
	}

	resp, err := c.client.Do(request)
	if err != nil {
		return err
	}

	defer func() {
		_, _ = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}()

	if c.trace != nil {
		respDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return fmt.Errorf("Error dumping response: %s", err)
		}

		_, err = c.trace.Write(append(respDump, []byte("\n--------------------\n\n")...))
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

func (c *Client) CreatePolicyForAppWithGUID(guid string, policy *Policy) error {
	req, err := c.newRequest(
		"POST",
		"/v1/apps/"+guid+"/policy",
		nil,
		policy,
	)
	if err != nil {
		return err
	}

	return c.doRequest(req, nil)
}
