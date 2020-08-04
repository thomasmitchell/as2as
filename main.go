package main

import (
	"fmt"
	"os"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	dumpCom := app.Command("dump", "Dump the autoscaling information out of the PCF server")
	cmdIndex["dump"] = &dumpCmd{
		ClientID:     dumpCom.Flag("client-id", "The client id to auth with").String(),
		ClientSecret: dumpCom.Flag("client-secret", "The client secret to auth with").String(),
		UAAHost:      dumpCom.Flag("uaa-host", "The UAA host to authenticate against").String(),
		PCFASHost:    dumpCom.Flag("pcfas-host", "The PCF Autoscaler API to talk to").String(),
		SpaceGUID:    dumpCom.Flag("space-guid", "(temp) the guid of the space to scrape").String(),
	}

	app.HelpFlag.Short('h')
	commandName := kingpin.MustParse(app.Parse(os.Args[1:]))
	cmd, found := cmdIndex[commandName]
	if !found {
		panic(fmt.Sprintf("Unregistered command %s", commandName))
	}

	err := cmd.Run()
	if err != nil {
		bailWith(err.Error())
	}
}

func bailWith(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "!! "+f+"\n", args...)
	os.Exit(1)
}
