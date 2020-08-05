package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/thomasmitchell/as2as/models"
	"github.com/thomasmitchell/as2as/ocfas"
)

type convertCmd struct {
	InputFile **os.File
}

func (c *convertCmd) Run() error {
	jDecoder := json.NewDecoder(*c.InputFile)

	dumpModel := models.Dump{}
	err := jDecoder.Decode(&dumpModel)
	if err != nil {
		return fmt.Errorf("Error decoding file input")
	}

	type PolicyToApp struct {
		GUID   string        `json:"guid"`
		Policy *ocfas.Policy `json:"policy,omitempty"`
	}

	type Space struct {
		GUID string        `json:"guid"`
		Apps []PolicyToApp `json:"apps,omitempty"`
	}

	type SpaceList struct {
		Spaces []Space `json:"spaces"`
	}

	output := SpaceList{}

	for _, space := range dumpModel.Spaces {
		appList := []PolicyToApp{}

		for _, app := range space.Apps {
			policy, err := app.ToOCFPolicy()
			if err != nil {
				return fmt.Errorf("Error constructing policy for app with GUID `%s' in space with GUID `%s': %s", app.GUID, space.GUID, err)
			}

			appList = append(appList, PolicyToApp{
				GUID:   app.GUID,
				Policy: policy,
			})
		}

		output.Spaces = append(output.Spaces,
			Space{
				GUID: space.GUID,
				Apps: appList,
			},
		)
	}

	jEncoder := json.NewEncoder(os.Stdout)
	jEncoder.SetIndent("", "  ")
	jEncoder.SetEscapeHTML(false)
	err = jEncoder.Encode(&output)
	if err != nil {
		return fmt.Errorf("Error encoding JSON to stdout: %s", err)
	}

	return nil
}
