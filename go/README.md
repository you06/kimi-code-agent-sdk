# kimi-code-agent-sdk for Go

Go client for the `kimi-code sdk-server --stdio` protocol.

```go
package main

import (
	"context"
	"fmt"

	kimi "github.com/you06/kimi-code-agent-sdk/go"
)

func main() {
	ctx := context.Background()
	client, err := kimi.Connect(ctx)
	if err != nil {
		panic(err)
	}
	defer client.Close(ctx)

	session, err := client.CreateSession(ctx, kimi.WithWorkDir("/path/to/project"))
	if err != nil {
		panic(err)
	}
	defer session.Close(ctx)

	events, err := session.Prompt(ctx, "Hello")
	if err != nil {
		panic(err)
	}
	for event := range events {
		if event.Type == "assistant.delta" {
			fmt.Print(event.Delta)
		}
	}
}
```

The client requires a `kimi-code` executable that supports:

```bash
kimi-code sdk-server --stdio
```

