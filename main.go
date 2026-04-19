package main

import (
	"fmt"
	"loreskills/cmd"
	"os"
)

func main() {
	if err := cmd.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		os.Exit(1)
	}
}
