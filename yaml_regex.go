package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"bufio"

	"gopkg.in/yaml.v3"

	"log"
	"time"
)

type Pattern struct {
	Pattern struct {
		Name       string `yaml:"name"`
		Regex      string `yaml:"regex"`
		Confidence string `yaml:"confidence"`
	} `yaml:"pattern"`
}

func readPatterns(filename string) ([]Pattern, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var patterns []Pattern
	err = yaml.Unmarshal(data, &patterns)
	if err != nil {
		return nil, err
	}

	return patterns, nil
}

func maskSecretKeys(input string, patterns []Pattern) string {
	// fmt.Println("Input string:", input)

	for _, pattern := range patterns {
		regex, err := regexp.Compile(pattern.Pattern.Regex)
		if err != nil {
			fmt.Printf("Error compiling regex '%s': %v\n", pattern.Pattern.Regex, err)
			continue
		}
		matches := regex.FindAllStringIndex(input, -1)
		for _, match := range matches {
			replacement := strings.Repeat("*", match[1]-match[0])
			input = input[:match[0]] + replacement + input[match[1]:]
			// fmt.Printf("Masked '%s' using regex '%s'\n", input[match[0]:match[1]], pattern.Pattern.Name)
		}
	}
	return input
}

// func maskSecretKeys(input string, patterns []Pattern) string {
// 	// Create a single regex expression by combining multiple patterns
// 	var regexExpressions []string
// 	for _, pattern := range patterns {
// 		regexExpressions = append(regexExpressions, pattern.Pattern.Regex)
// 	}
// 	combinedRegex := strings.Join(regexExpressions, "|")

// 	// Compile the combined regex expression
// 	regex, err := regexp.Compile(combinedRegex)
// 	if err != nil {
// 		fmt.Printf("Error compiling regex '%s': %v\n", combinedRegex, err)
// 		return input
// 	}

// 	// Find all matches using the combined regex expression
// 	matches := regex.FindAllStringIndex(input, -1)

// 	// Replace the matched strings with asterisks
// 	for _, match := range matches {
// 		replacement := strings.Repeat("*", match[1]-match[0])
// 		input = input[:match[0]] + replacement + input[match[1]:]
// 	}

// 	return input
// }

func main() {
	patterns, err := readPatterns("patterns.yaml")
	if err != nil {
		fmt.Printf("Error reading patterns: %v\n", err)
		return
	}

	// file, err := os.Open("main.logs")
	file, err := os.Open("synthetic_log_data.txt")
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	// Create a buffered reader
	reader := bufio.NewReader(file)

	// Start the timer
	start := time.Now()

	// Read and print the log data line by line
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		// input := "From: https://huggingface.co/spaces/presidio/presidio_demo Here are a few example sentences we currently support: Hello, my name is David Johnson and I live in Maine.My credit card number is 4095-2609-9393-4932 and my crypto wallet id is 16Yeky6GMjeNkAiNcBY7ZhrLoMSgg1BoyZ.On September 18 I visited microsoft.com and sent an email to test@presidio.site,  from the IP 192.168.0.1.My passport: 191280342 and my phone number: (212) 555-1234.This is a valid International Bank Account Number: IL150120690000003111111 . Can you please check the status on bank account 954567876544?Kate's social security number is 078-05-1126.  Her driver license? it is 1234567A.This is a test string with a Slack Token: xoxp-12345678-12345678-12345678-abcdef0123456789 and an RSA private key: -----BEGIN RSA PRIVATE KEY-----..."

		// maskedString := maskSecretKeys(input, patterns)
		maskedString := maskSecretKeys(line, patterns)
		fmt.Println("Masked string:", maskedString)
	}

	// Calculate the execution time
	duration := time.Since(start)
	fmt.Printf("\nExecution time: %v\n", duration)

}
