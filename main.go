package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/doomsday-project/doomsday/storage/uaa"
	"github.com/thomasmitchell/as2as/models"
	"github.com/thomasmitchell/as2as/pcfas"
)

func main() {
	var (
		uaaHost         = mustEnv("UAA_HOST")
		pcfClientID     = mustEnv("PCF_CLIENT_ID")
		pcfClientSecret = mustEnv("PCF_CLIENT_SECRET")
		pcfasHost       = mustEnv("PCFAS_HOST")
		spaceGUID       = mustEnv("SPACE_GUID")
	)

	u := url.URL{
		Scheme: "https",
		Host:   uaaHost,
	}

	uaaClient := uaa.Client{
		URL: u.String(),
	}

	tokenResp, err := uaaClient.ClientCredentials(pcfClientID, pcfClientSecret)
	if err != nil {
		bailWith("Error retrieving auth token: %s", err)
	}

	token := tokenResp.AccessToken

	pcfasClient := pcfas.NewClient(pcfasHost, token)
	pcfasClient.TraceTo(os.Stderr)

	appsForSpace, err := pcfasClient.AppsForSpaceWithGUID(spaceGUID)
	if err != nil {
		bailWith("Error getting apps: %s", err)
	}

	var modelApps []models.App
	for i := range appsForSpace {
		rules, err := pcfasClient.RulesForAppWithGUID(appsForSpace[i].GUID)
		if err != nil {
			bailWith("Error getting rules for app with GUID `%s': %s",
				appsForSpace[i].GUID,
				err,
			)
		}

		scheduledLimitChanges, err := pcfasClient.ScheduledLimitChangesForAppWithGUID(
			appsForSpace[i].GUID,
		)
		if err != nil {
			bailWith("Error getting scheduled limit changes for app with GUID `%s': %s",
				appsForSpace[i].GUID,
				err,
			)
		}
		thisModelApp, err := models.ConstructApp(
			spaceGUID,
			appsForSpace[i],
			rules,
			scheduledLimitChanges,
		)
		if err != nil {
			fmt.Errorf("Error transforming app data to intermediate representation: %s", err)
		}

		modelApps = append(modelApps, thisModelApp)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(modelApps)
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
