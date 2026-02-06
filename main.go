package main

import (
	"log"
	"stackbill-deployer/cmd/server"
)

func main() {
	log.Println("Starting StackBill Deployer...")
	server.Run()
}
