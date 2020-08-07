package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

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
	cf, err := buildCFClient(*d.CFHost, *d.ClientID, *d.ClientSecret)
	if err != nil {
		return err
	}

	errChan := make(chan error)

	spaceGUIDChan, err := d.fetchSpaceGUIDsToScrape(cf, errChan)
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

	outputSpaceChan := make(chan models.Space, 10)

	doneChan := make(chan bool)
	const numWorkers = 8

	fmt.Fprintf(os.Stderr, "Scraping autoscaler for all known apps\n")
	scrapeWait := sync.WaitGroup{}
	scrapeWait.Add(numWorkers)

	scrapeSpaces := func() {
		for spaceGUID := range spaceGUIDChan {
			appsForSpace, err := pcfasClient.AppsForSpaceWithGUID(spaceGUID)
			if err != nil {
				errChan <- fmt.Errorf("Error getting apps for space with GUID `%s': %s", spaceGUID, err)
			}

			var modelApps []models.App
			for j := range appsForSpace {
				thisModelApp, err := d.scrapeApp(appsForSpace[j], pcfasClient)
				if err != nil {
					errChan <- err
				}
				modelApps = append(modelApps, thisModelApp)
			}

			outputSpaceChan <- models.Space{
				GUID: spaceGUID,
				Apps: modelApps,
			}
		}

		scrapeWait.Done()
	}
	for i := 0; i < numWorkers; i++ {
		go scrapeSpaces()
	}
	go func() {
		scrapeWait.Wait()
		fmt.Fprintf(os.Stderr, "All space scrape workers done\n")
		close(outputSpaceChan)
	}()

	outputDump := &models.Dump{}

	go func() {
		for space := range outputSpaceChan {
			outputDump.Spaces = append(outputDump.Spaces, space)
		}

		fmt.Fprintf(os.Stderr, "Output builder done\n")
		doneChan <- true
	}()

	select {
	case err := <-errChan:
		return err

	case <-doneChan:
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	err = enc.Encode(&outputDump)
	if err != nil {
		return fmt.Errorf("Could not encode JSON: %s", err)
	}

	return nil
}

func (d *dumpCmd) fetchSpaceGUIDsToScrape(cf *cfclient.Client, errChan chan<- error) (<-chan string, error) {
	const numWorkers = 4
	fmt.Fprintf(os.Stderr, "Listing plans for broker with GUID `%s'\n", *d.BrokerGUID)
	servicesQuery := url.Values{}
	servicesQuery.Add("q", fmt.Sprintf("service_broker_guid:%s", *d.BrokerGUID))
	plansForBroker, err := cf.ListServicePlansByQuery(servicesQuery)
	if err != nil {
		return nil, fmt.Errorf("Error listing service plans for broker with GUID `%s': %s", *d.BrokerGUID, err)
	}

	var allServiceInstances []cfclient.ServiceInstance
	for _, plan := range plansForBroker {
		serviceInstancesQuery := url.Values{}
		serviceInstancesQuery.Add("q", "service_plan_guid:"+plan.Guid)
		serviceInstances, err := cf.ListServiceInstancesByQuery(serviceInstancesQuery)
		if err != nil {
			return nil, fmt.Errorf("Error listing service instances for plan `%s': %s", plan.Guid, err)
		}

		allServiceInstances = append(allServiceInstances, serviceInstances...)
	}

	validSpacesChan := make(chan string, len(allServiceInstances))
	serviceInstanceChan := make(chan cfclient.ServiceInstance, len(allServiceInstances))

	go func() {
		for _, serviceInstance := range allServiceInstances {
			serviceInstanceChan <- serviceInstance
		}

		close(serviceInstanceChan)
	}()

	wait := sync.WaitGroup{}
	wait.Add(numWorkers)

	fmt.Fprintf(os.Stderr, "Querying service bindings\n")
	for i := 0; i < numWorkers; i++ {
		go func() {
			for serviceInstance := range serviceInstanceChan {
				bindingsQuery := url.Values{}
				bindingsQuery.Add("q", "service_instance_guid:"+serviceInstance.Guid)
				bindings, err := cf.ListServiceBindingsByQuery(bindingsQuery)
				if err != nil {
					errChan <- fmt.Errorf("Error checking service bindings for service instance with GUID `%s': %s", serviceInstance.Guid, err)
					return
				}

				if len(bindings) > 0 {
					validSpacesChan <- serviceInstance.SpaceGuid
				}
			}

			wait.Done()
		}()
	}

	go func() {
		wait.Wait()
		fmt.Fprintf(os.Stderr, "Done querying service bindings\n")
		close(validSpacesChan)
	}()

	return validSpacesChan, nil
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
