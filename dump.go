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
	BrokerGUID   *string
}

func (d *dumpCmd) Run() error {
	cf, err := d.buildCFClient()
	if err != nil {
		return err
	}

	spaceGUIDsToScrape, err := d.fetchSpaceGUIDsToScrape(cf)
	if err != nil {
		return err
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

	fmt.Fprintf(os.Stderr, "Scraping autoscaler for all known apps\n")
	outputDump := &models.Dump{}
	for _, spaceGUID := range spaceGUIDsToScrape {
		appsForSpace, err := pcfasClient.AppsForSpaceWithGUID(spaceGUID)
		if err != nil {
			return fmt.Errorf("Error getting apps for space with GUID `%s': %s", spaceGUID, err)
		}

		var modelApps []models.App
		for j := range appsForSpace {
			thisModelApp, err := d.scrapeApp(appsForSpace[j], pcfasClient)
			if err != nil {
				return err
			}
			modelApps = append(modelApps, thisModelApp)
		}

		outputDump.Spaces = append(outputDump.Spaces, models.Space{
			GUID: spaceGUID,
			Apps: modelApps,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(&outputDump)
	if err != nil {
		return fmt.Errorf("Could not encode JSON: %s", err)
	}

	return nil
}

func (d *dumpCmd) buildCFClient() (*cfclient.Client, error) {
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

	fmt.Fprintf(os.Stderr, "Authing to CF\n")

	ret, err := cfclient.NewClient(cfClientConfig)
	if err != nil {
		return nil, fmt.Errorf("Error initializing CF client: %s")
	}

	return ret, nil
}

func (d *dumpCmd) fetchSpaceGUIDsToScrape(cf *cfclient.Client) ([]string, error) {
	fmt.Fprintf(os.Stderr, "Listing services for broker with GUID `%s'\n", *d.BrokerGUID)
	servicesQuery := url.Values{}
	servicesQuery.Add("q", fmt.Sprintf("service_broker_guid:%s", *d.BrokerGUID))
	servicesForBroker, err := cf.ListServicesByQuery(servicesQuery)
	if err != nil {
		return nil, fmt.Errorf("Error list services for broker with GUID `%s': %s", *d.BrokerGUID, err)
	}

	var targetedServiceGUIDs StringList
	for i := range servicesForBroker {
		targetedServiceGUIDs = append(targetedServiceGUIDs, servicesForBroker[i].Guid)
	}

	fmt.Fprintf(os.Stderr, "Listing spaces in CF\n")
	allSpaces, err := cf.ListSpaces()
	if err != nil {
		return nil, fmt.Errorf("Error listing spaces in CF: %s", err)
	}

	fmt.Fprintf(os.Stderr, "Querying spaces for autoscaler service instances with offering GUIDs %+v\n", targetedServiceGUIDs)
	var ret []string
	for i := range allSpaces {
		serviceInstancesQuery := url.Values{}
		serviceInstancesQuery.Add("q", fmt.Sprintf("space_guid:%s", allSpaces[i].Guid))
		serviceInstances, err := cf.ListServiceInstancesByQuery(serviceInstancesQuery)
		if err != nil {
			return nil, fmt.Errorf("Error getting service instances in space with GUID `%s': %s", allSpaces[i].Guid, err)
		}

		for j := range serviceInstances {
			if targetedServiceGUIDs.Contains(serviceInstances[j].ServiceGuid) {
				ret = append(ret, allSpaces[i].Guid)
				break
			}
		}
	}

	return ret, nil
}

func (d *dumpCmd) scrapeApp(app pcfas.App, pcfasClient *pcfas.Client) (models.App, error) {
	ret := models.App{}
	rules, err := pcfasClient.RulesForAppWithGUID(app.GUID)
	if err != nil {
		return ret,
			fmt.Errorf("Error getting rules for app with GUID `%s': %s",
				app.GUID,
				err,
			)
	}

	scheduledLimitChanges, err := pcfasClient.ScheduledLimitChangesForAppWithGUID(
		app.GUID,
	)
	if err != nil {
		return ret,
			fmt.Errorf("Error getting scheduled limit changes for app with GUID `%s': %s",
				app.GUID,
				err,
			)
	}
	ret, err = models.ConstructApp(app, rules, scheduledLimitChanges)
	if err != nil {
		return ret,
			fmt.Errorf("Error transforming app data to intermediate representation: %s", err)
	}

	return ret, nil
}
