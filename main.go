package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/doomsday-project/doomsday/storage/uaa"
	"github.com/thomasmitchell/as2as/pcfas"
)

func main() {
	uaaHost := mustEnv("UAA_HOST")

	u := url.URL{
		Scheme: "https",
		Host:   uaaHost,
	}

	uaaClient := uaa.Client{
		URL: u.String(),
	}

	clientID := mustEnv("CLIENT_ID")
	clientSecret := mustEnv("CLIENT_SECRET")

	tokenResp, err := uaaClient.ClientCredentials(clientID, clientSecret)
	if err != nil {
		bailWith("Error retrieving auth token: %s", err)
	}

	token := tokenResp.AccessToken

	pcfasHost := mustEnv("PCFAS_HOST")
	pcfasClient := pcfas.NewClient(pcfasHost, token)
	pcfasClient.TraceTo(os.Stderr)

	spaceGUID := mustEnv("SPACE_GUID")
	appsForSpace, err := pcfasClient.AppsForSpaceWithGUID(spaceGUID)
	if err != nil {
		bailWith("Error getting apps: %s", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(appsForSpace)
	if err != nil {
		bailWith("Could not encode JSON: %s", err)
	}
}

func mustEnv(envvar string) string {
	ret := os.Getenv(envvar)
	if ret == "" {
		bailWith("%s must be set", envvar)
	}

	return ret
}

func bailWith(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "!! "+f+"\n", args...)
	os.Exit(1)
}
