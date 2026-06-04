package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
)

func simplify(v interface{}) interface{} {
	switch vv := v.(type) {
	case map[string]interface{}:
		for k, val := range vv {
			vv[k] = simplify(val)
		}
		return vv
	case []interface{}:
		if len(vv) > 0 {
			// Ambil hanya data pertama
			return []interface{}{simplify(vv[0])}
		}
		return vv
	default:
		return vv
	}
}

func main() {
	inputPath := "trade-book.json"
	outputPath := "trade-book-simple.json"

	data, err := ioutil.ReadFile(inputPath)
	if err != nil {
		log.Fatal(err)
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		log.Fatal(err)
	}

	simplified := simplify(result)

	out, err := json.MarshalIndent(simplified, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	if err := ioutil.WriteFile(outputPath, out, 0644); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Successfully simplified %s to %s\n", inputPath, outputPath)
}
