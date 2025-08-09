package main

import "github.com/k-butz/c8y-az-eventhub-connector/cmd/app"

func main() {
	runtimeApp := app.NewApp()
	runtimeApp.Run()
}
