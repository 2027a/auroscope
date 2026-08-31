package main

import (
	"os"

	"git.2027a.net/2027a/auroscope/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
