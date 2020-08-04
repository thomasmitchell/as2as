package main

import "gopkg.in/alecthomas/kingpin.v2"

type command interface {
	Run() error
}

var app = kingpin.New("as2as", "PCF Autoscaler to OCF Autoscaler Migration Tool")
var cmdIndex = map[string]command{}
var globalTrace = app.Flag("trace", "Show HTTP trace").Short('T').Bool()
