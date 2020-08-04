package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/thomasmitchell/as2as/models"
	"github.com/thomasmitchell/as2as/pcfas"
)

type dumpCmd struct {
	ClientID     *string
	ClientSecret *string
	CFHost       *string
	PCFASHost    *string
	SpaceGUID    *string
}

func (d *dumpCmd) Run() error {
	u := url.URL{
		Scheme: "https",
		Host:   *d.CFHost,
	}

	cfClientConfig := &cfclient.Config{
		ApiAddress:   u.String(),
		ClientID:     *d.ClientID,
		ClientSecret: *d.ClientSecret,
		UserAgent:    "Go-CF-client/1.1",
	}

	cf, err := cfclient.NewClient(cfClientConfig)
	if err != nil {
		return fmt.Errorf("Error initializing CF client: %s")
	}

	token, err := cf.GetToken()
	if err != nil {
		return fmt.Errorf("Error retrieving auth token: %s")
	}

	token = strings.TrimPrefix(token, "bearer ")

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
	err = enc.Encode(
		&models.Dump{
			Spaces: []models.Space{
				{
					GUID: *d.SpaceGUID,
					Apps: modelApps,
				},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("Could not encode JSON: %s", err)
	}

	return nil
}
