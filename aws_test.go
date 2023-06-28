package main

import (
	"bufio"
	"os"
	"testing"
)

func TestFilenameText(t *testing.T) {
	sampleFile, _ := os.Open("testdata/egg.txt")
	expectedName := "6ziwppLVo8-9ZA4RddxHhHWIXCznXwcVJmMDLSQhg7Y/egg.txt"
	newName, err := Filename("egg.txt", bufio.NewReader(sampleFile))

	if expectedName != newName || err != nil {
		t.Fatalf(
			`Filename("test.txt", io.Reader)) = %s, %v, want %s`,
			newName, err, expectedName,
		)
	}
}

func TestFilenameBinary(t *testing.T) {
	sampleFile, _ := os.Open("testdata/smol.gif")
	expectedName := "IoFqAN_p_NwwBj0icXq5y6s66yqOmETp13TSVtxIt8g/smol.gif"
	newName, err := Filename("smol.gif", bufio.NewReader(sampleFile))

	if expectedName != newName || err != nil {
		t.Fatalf(
			`Filename("smol.gif", io.Reader)) = %s, %v, want %s`,
			newName, err, expectedName,
		)
	}
}
