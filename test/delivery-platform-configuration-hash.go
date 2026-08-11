package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	var hash string
	switch mode() {
	case "intent":
		var value delivery.DeliveryIntent
		if err := json.Unmarshal(data, &value); err != nil {
			panic(err)
		}
		hash, err = value.ComputeCanonicalHash()
	case "platform":
		var value delivery.PlatformConfiguration
		if err := json.Unmarshal(data, &value); err != nil {
			panic(err)
		}
		hash, err = value.ComputeCanonicalHash()
	default:
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			panic(err)
		}
		hash, err = contract.CanonicalJSONHash(value)
	}
	if err != nil {
		panic(err)
	}

	fmt.Print(hash)
}

func mode() string {
	if len(os.Args) < 2 {
		return "raw"
	}
	return os.Args[1]
}
