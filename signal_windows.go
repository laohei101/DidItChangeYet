//go:build windows

package main

import (
	"os"
	"os/signal"
)

func waitForShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
