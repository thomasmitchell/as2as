package main

import (
	"fmt"
	"os"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	dumpCom := app.Command("dump", "Dump the autoscaling information out of the PCF server")
	cmdIndex["dump"] = &dumpCmd{
		ClientID:     dumpCom.Flag("client-id", "The client id to auth with").Required().String(),
		ClientSecret: dumpCom.Flag("client-secret", "The client secret to auth with").Required().String(),
		CFHost:       dumpCom.Flag("cf-host", "The CF API host to scrape from").Required().String(),
		PCFASHost:    dumpCom.Flag("pcfas-host", "The PCF Autoscaler API to talk to").Required().String(),
		BrokerGUID:   dumpCom.Flag("broker-guid", "The GUID of the autoscaler service broker").Required().String(),
	}

	convertCom := app.Command("convert", "Display OCF autoscaler converted rules")
	cmdIndex["convert"] = &convertCmd{
		InputFile: convertCom.Flag("input-file", "The file to read the exported data from").Short('f').Required().File(),
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
