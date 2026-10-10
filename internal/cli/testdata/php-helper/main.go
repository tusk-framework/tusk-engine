package main

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
)

func main() {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(90)
	}
	result := struct {
		Args  []string `json:"args"`
		Dir   string   `json:"dir"`
		Stdin string   `json:"stdin"`
	}{Args: os.Args[1:], Stdin: string(stdin)}
	result.Dir, err = os.Getwd()
	if err != nil {
		os.Exit(91)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(92)
	}
	_, _ = io.WriteString(os.Stderr, "child stderr")
	exitCode, err := strconv.Atoi(os.Getenv("TUSK_TEST_PHP_EXIT_CODE"))
	if err != nil {
		os.Exit(93)
	}
	os.Exit(exitCode)
}
