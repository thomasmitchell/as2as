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

type dumpCmd struct {
	ClientID     *string
	ClientSecret *string
	UAAHost      *string
	PCFASHost    *string
	SpaceGUID    *string
}

func (d *dumpCmd) Run() error {
	u := url.URL{
		Scheme: "https",
		Host:   *d.UAAHost,
	}

	uaaClient := uaa.Client{
		URL: u.String(),
	}

	tokenResp, err := uaaClient.ClientCredentials(*d.ClientID, *d.ClientSecret)
	if err != nil {
		return fmt.Errorf("Error retrieving auth token: %s", err)
	}

	token := tokenResp.AccessToken

	pcfasClient := pcfas.NewClient(*d.PCFASHost, token)
	if globalTrace != nil && *globalTrace {
		pcfasClient.TraceTo(os.Stderr)
	}

	appsForSpace, err := pcfasClient.AppsForSpaceWithGUID(*d.SpaceGUID)
	if err != nil {
		return fmt.Errorf("Error getting apps: %s", err)
	}

	var modelApps []models.App
	for i := range appsForSpace {
		rules, err := pcfasClient.RulesForAppWithGUID(appsForSpace[i].GUID)
		if err != nil {
			return fmt.Errorf("Error getting rules for app with GUID `%s': %s",
				appsForSpace[i].GUID,
				err,
			)
		}

		scheduledLimitChanges, err := pcfasClient.ScheduledLimitChangesForAppWithGUID(
			appsForSpace[i].GUID,
		)
		if err != nil {
			return fmt.Errorf("Error getting scheduled limit changes for app with GUID `%s': %s",
				appsForSpace[i].GUID,
				err,
			)
		}
		thisModelApp, err := models.ConstructApp(
			*d.SpaceGUID,
			appsForSpace[i],
			rules,
			scheduledLimitChanges,
		)
		if err != nil {
			return fmt.Errorf("Error transforming app data to intermediate representation: %s", err)
		}

		modelApps = append(modelApps, thisModelApp)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(modelApps)
	if err != nil {
		return fmt.Errorf("Could not encode JSON: %s", err)
	}

	return nil
}
