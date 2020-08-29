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
	"github.com/thomasmitchell/as2as/ocfas"
)

type syncCmd struct {
	InputFile           **os.File
	ClientID            *string
	ClientSecret        *string
	CFHost              *string
	OCFASHost           *string
	BrokerGUID          *string
	ServiceInstanceName *string
	Workers             *int
}

func (s *syncCmd) Run() error {
	jDecoder := json.NewDecoder(*s.InputFile)
	syncInput := models.Converted{}
	err := jDecoder.Decode(&syncInput)
	if err != nil {
		return fmt.Errorf("Error parsing input file JSON: %s", err)
	}

	err = (*s.InputFile).Close()
	if err != nil {
		return fmt.Errorf("Error closing input file")
	}

	cf, err := buildCFClient(*s.CFHost, *s.ClientID, *s.ClientSecret)
	if err != nil {
		return err
	}

	planGUIDs, err := s.getServicePlanGUIDs(cf)
	if err != nil {
		return err
	}

	if len(planGUIDs) == 0 {
		return fmt.Errorf("No service plans exist for service broker with GUID `%s'", *s.BrokerGUID)
	}

	spacesToInstances, err := s.mapSpaceGUIDsToServiceInstances(cf, planGUIDs)
	if err != nil {
		return err
	}

	spacesToCreateInstances := make(chan models.ConvertedSpace, len(syncInput.Spaces))
	go func() {
		for i := range syncInput.Spaces {
			spacesToCreateInstances <- syncInput.Spaces[i]
		}

		close(spacesToCreateInstances)
	}()

	numWorkers := *(s.Workers)

	instancesWaitGroup := sync.WaitGroup{}
	instancesWaitGroup.Add(numWorkers)
	readySpacesChan := make(chan SyncServiceInstanceSpacePair, len(syncInput.Spaces))
	errChan := make(chan error)
	fmt.Fprintf(os.Stderr, "Creating service instances\n")
	for i := 0; i < numWorkers; i++ {
		go s.createServiceInstancesForSpaces(
			cf,
			spacesToCreateInstances,
			readySpacesChan,
			&instancesWaitGroup,
			errChan,
			*s.ServiceInstanceName,
			planGUIDs[0],
			spacesToInstances,
		)
	}
	go func() {
		instancesWaitGroup.Wait()
		fmt.Fprintf(os.Stderr, "Done creating service instances\n")
		close(readySpacesChan)
	}()

	bindAppsWaitGroup := sync.WaitGroup{}
	bindAppsWaitGroup.Add(numWorkers)

	fmt.Fprintf(os.Stderr, "Binding services to apps\n")
	appChan := make(chan models.ConvertedPolicyToApp, 1000)
	for i := 0; i < numWorkers; i++ {
		go s.bindServiceToApps(
			cf,
			readySpacesChan,
			appChan,
			&bindAppsWaitGroup,
			errChan,
		)
	}
	go func() {
		bindAppsWaitGroup.Wait()
		fmt.Fprintf(os.Stderr, "Done binding services to apps\n")
		close(appChan)
	}()

	setPoliciesWaitGroup := sync.WaitGroup{}
	setPoliciesWaitGroup.Add(numWorkers)
	doneChan := make(chan bool)
	token, err := cf.GetToken()
	if err != nil {
		return fmt.Errorf("Error retrieving auth token: %s", err)
	}
	as := ocfas.NewClient(*s.OCFASHost, strings.TrimPrefix(token, "bearer "))
	if globalTrace != nil && *globalTrace {
		as.TraceTo(os.Stderr)
	}

	fmt.Fprintf(os.Stderr, "Setting policies on apps\n")
	for i := 0; i < numWorkers; i++ {
		go s.setAppPolicies(
			as,
			appChan,
			&setPoliciesWaitGroup,
			errChan,
		)
	}
	go func() {
		setPoliciesWaitGroup.Wait()
		fmt.Fprintf(os.Stderr, "Done setting policies on apps\n")
		doneChan <- true
	}()

	select {
	case err := <-errChan:
		return err
	case <-doneChan:
		fmt.Fprintf(os.Stderr, "Done!\n")
		return nil
	}
}

func (s *syncCmd) getServicePlanGUIDs(cf *cfclient.Client) ([]string, error) {
	fmt.Fprintf(os.Stderr, "Checking if service broker with GUID `%s' exists\n", *s.BrokerGUID)
	_, err := cf.GetServiceBrokerByGuid(*s.BrokerGUID)
	if err != nil {
		return nil, fmt.Errorf("Error discovering service broker `%s'", err)
	}

	fmt.Fprintf(os.Stderr, "Looking up service plans for service broker with GUID `%s'\n", *s.BrokerGUID)
	//Discover which spaces have service instances of the proper type bound
	servicePlansQuery := url.Values{}
	servicePlansQuery.Add("q", "service_broker_guid:"+*s.BrokerGUID)
	plans, err := cf.ListServicePlansByQuery(servicePlansQuery)
	if err != nil {
		return nil, fmt.Errorf("Error listing service plans for broker with GUID `%s': %s", *s.BrokerGUID, err)
	}

	ret := []string{}
	for _, plan := range plans {
		ret = append(ret, plan.Guid)
	}

	return ret, nil
}

func (s *syncCmd) mapSpaceGUIDsToServiceInstances(cf *cfclient.Client, planGUIDs []string) (map[string]string, error) {
	//space_guid -> service_instance_guid
	spaceInstanceLookup := map[string]string{}

	for _, planGUID := range planGUIDs {
		fmt.Fprintf(os.Stderr, "Looking up service instances for service plan with GUID `%s'\n", planGUID)
		serviceInstancesQuery := url.Values{}
		serviceInstancesQuery.Add("q", "service_plan_guid:"+planGUID)
		serviceInstances, err := cf.ListServiceInstancesByQuery(serviceInstancesQuery)
		if err != nil {
			return nil, fmt.Errorf("Error listing service instances for plan `%s': %s", planGUID, err)
		}

		for _, serviceInstance := range serviceInstances {
			spaceInstanceLookup[serviceInstance.SpaceGuid] = serviceInstance.Guid
		}
	}

	return spaceInstanceLookup, nil
}

func (s *syncCmd) createServiceInstancesForSpaces(
	cf *cfclient.Client,
	spaces <-chan models.ConvertedSpace,
	output chan<- SyncServiceInstanceSpacePair,
	done *sync.WaitGroup,
	errChan chan<- error,
	instanceName string,
	servicePlanGUID string,
	spacesToInstances map[string]string,
) {
	for space := range spaces {
		serviceInstanceGUID, hasInstance := spacesToInstances[space.GUID]

		if !hasInstance {
			//create the service instance
			serviceInstance, err := cf.CreateServiceInstance(cfclient.ServiceInstanceRequest{
				Name:            instanceName,
				SpaceGuid:       space.GUID,
				ServicePlanGuid: servicePlanGUID,
			})
			if err != nil {
				errChan <- fmt.Errorf("Error when creating service instance of plan with GUID `%s' in space with GUID `%s': %s",
					servicePlanGUID, space.GUID, err)
				return
			}

			serviceInstanceGUID = serviceInstance.Guid
		}

		output <- SyncServiceInstanceSpacePair{
			Space:               space,
			ServiceInstanceGUID: serviceInstanceGUID,
		}
	}

	done.Done()
}

type SyncServiceInstanceSpacePair struct {
	Space               models.ConvertedSpace
	ServiceInstanceGUID string
}

func (s *syncCmd) bindServiceToApps(
	cf *cfclient.Client,
	spaces <-chan SyncServiceInstanceSpacePair,
	output chan models.ConvertedPolicyToApp,
	done *sync.WaitGroup,
	errChan chan<- error,
) {
	for spacePair := range spaces {
		for _, app := range spacePair.Space.Apps {
			//check if binding exists
			bindingsQuery := url.Values{}
			bindingsQuery.Add("q", "app_guid:"+app.GUID)
			bindingsQuery.Add("q", "service_instance_guid:"+spacePair.ServiceInstanceGUID)
			bindings, err := cf.ListServiceBindingsByQuery(bindingsQuery)
			if err != nil {
				errChan <- fmt.Errorf("Error checking service bindings for app with GUID `%s': %s", app.GUID, err)
				return
			}

			if len(bindings) == 0 {
				_, err = cf.CreateServiceBinding(app.GUID, spacePair.ServiceInstanceGUID)
				if err != nil {
					errChan <- fmt.Errorf("Error binding service instance with GUID `%s' to app with GUID `%s': %s",
						spacePair.ServiceInstanceGUID, app.GUID, err)
				}
			}

			output <- app
		}
	}

	done.Done()
}

func (s *syncCmd) setAppPolicies(
	as *ocfas.Client,
	apps <-chan models.ConvertedPolicyToApp,
	done *sync.WaitGroup,
	errChan chan<- error,
) {
	for app := range apps {
		err := as.CreatePolicyForAppWithGUID(app.GUID, app.Policy)
		if err != nil {
			errChan <- fmt.Errorf("Error when creating policy for app with GUID `%s': %s", app.GUID, err)
		}
	}

	done.Done()
}
