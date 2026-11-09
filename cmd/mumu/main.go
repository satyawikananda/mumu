// Command mumu runs the native macOS TV player.
package main

import (
	"log"

	"mumu/internal/app"
)

func main() {
	if err := app.New().Run(); err != nil {
		log.Fatal(err)
	}
}
