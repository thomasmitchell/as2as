package main

import (
	"fmt"
	"net/url"
	"os"

	"github.com/cloudfoundry-community/go-cfclient"
)

type StringList []string

func (l StringList) Contains(s string) bool {
	for i := range l {
		if l[i] == s {
			return true
		}
	}

	return false
}

func buildCFClient(host, clientID, clientSecret string) (*cfclient.Client, error) {
	u := url.URL{
		Scheme: "https",
		Host:   host,
	}

	cfClientConfig := &cfclient.Config{
		ApiAddress:   u.String(),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		UserAgent:    "Go-CF-client/1.1",
	}

	fmt.Fprintf(os.Stderr, "Authing to CF\n")

	ret, err := cfclient.NewClient(cfClientConfig)
	if err != nil {
		return nil, fmt.Errorf("Error initializing CF client: %s")
	}

	return ret, nil
}
