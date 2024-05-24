package main_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"main"
	"math/big"
	"os"
	"testing"
)

func BenchmarkSecrets(b *testing.B) {
	b.N = 100000
	for i := 0; i < b.N; i++ {
		main.MaskSecretsOnString("This is a log entry with a secret_key=9JHQpcS6HLVI8NyiMNsIRyLCw15lRQ", secret.BuiltinRules)
	}
}

func BenchmarkFileSecrets(b *testing.B) {
	file, err := os.Open("message.txt")
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var outBuf bytes.Buffer
	_, err = io.Copy(&outBuf, file)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}
	main.MaskSecretsStream(&outBuf)

}
func BenchmarkRandInt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rand.Int(rand.Reader, big.NewInt(10000))
	}
}
