# GoDatabase

GoDatabase provides a string-based key-value API backed by either an in-memory
map or a durable, page-backed B+Tree.

## Usage

```go
database, err := db.Open("app.db")
if err != nil {
	log.Fatal(err)
}
defer database.Close()

if err := database.Set("language", "Go"); err != nil {
	log.Fatal(err)
}

value, found, err := database.Get("language")
```

`Open` creates or reopens durable storage. The caller owns the returned database
and must call `Close`. `New` remains available for temporary in-memory storage.
