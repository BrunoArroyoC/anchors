package main

import (
	"log"

	"github.com/moby/moby/client"
)

func main() {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	//Functions
}
