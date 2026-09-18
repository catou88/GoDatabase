# GoDatabase

GoDatabase provides a string-based key-value API backed by either an in-memory
map or a durable, page-backed B+Tree.

## Usage

```go
package main

import (
	"fmt"
	"log"

	"godatabase/db"
)

func main() {
	database, err := db.Open("app.db")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	if err := database.Set("language", "Go"); err != nil {
		log.Fatal(err)
	}

	value, found, err := database.Get("language")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(value, found)
}
```

`Open` creates or reopens durable storage. The caller owns the returned database
and must call `Close`. `New` remains available for temporary in-memory storage.

See [Durable Storage](docs/durable-storage.md) for API semantics, durability and
recovery guarantees, free-page reuse, compatibility, and known limitations.
