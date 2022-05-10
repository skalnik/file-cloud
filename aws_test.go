package main

import (
	"bufio"
	"os"
	"testing"
)

func TestFilename(t *testing.T) {
	sampleFile, err := os.Open("testdata/egg.txt")
	expectedName := "6ziwppLVo8-9ZA4RddxHhHWIXCznXwcVJmMDLSQhg7Y/egg.txt"
	newName, err := Filename("egg.txt", bufio.NewReader(sampleFile))

	if expectedName != newName || err != nil {
		t.Fatalf(
			`Filename("test.txt", test)) = %s, %v, want %s`,
			newName, err, expectedName,
		)
	}
}
