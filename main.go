package main

import (
	"log"

	"github.com/gunasekar/cryptowatch/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
