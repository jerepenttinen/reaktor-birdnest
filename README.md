# Project Birdnest
Reaktor [pre-assignment](https://assignments.reaktor.com/birdnest) solution.

Deployed [here](https://jere-birdnest.fly.dev/).

### Approach

Poll the API every 2 seconds, render an HTML template and send it to the clients via server-sent events.

Pilot information is persisted using Redis or alternatively in a queue that is in insertion/update order.

To use Redis set `REDIS_URL` environment variable.

### Important files
* `cmd/api/monitor.go` Event loop that drives the application
* `cmd/api/monitor_test.go` Tests for the previous
* `cmd/api/main.go` Setup code for the application
* `internal/persistence/myredis/myredis.go` Persistence using Redis
* `internal/persistence/datastore/datastore.go` Queue for persisting the pilot information
* `internal/models/birdnest/birdnest.go` Repository for the assignment API
