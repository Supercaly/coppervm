package main

import (
	"fmt"
	"io"
	"log"
	"os"

	au "github.com/Supercaly/coppervm/internal"
	"github.com/Supercaly/coppervm/pkg/coppervm"
)

func usage(stream io.Writer, program string) {
	fmt.Fprintf(stream, "Usage: %s [OPTIONS] <input.vm>\n", program)
	fmt.Fprintf(stream, "[OPTIONS]: \n")
	fmt.Fprintf(stream, "    -m     Print the program memory to stdout.\n")
	fmt.Fprintf(stream, "    -l     Print the line number before the line.\n")
	fmt.Fprintf(stream, "    -h     Print this help message.\n")
}

func main() {
	args := os.Args
	var program string
	program, args = au.Shift(args)
	printMemory := false
	printLineNbr := false
	var inputFilePath string

	for len(args) > 0 {
		var flag string
		flag, args = au.Shift(args)

		if flag == "-h" {
			usage(os.Stdout, program)
			os.Exit(0)
		} else if flag == "-m" {
			printMemory = true
		} else if flag == "-l" {
			printLineNbr = true
		} else {
			if inputFilePath != "" {
				usage(os.Stderr, program)
				log.Fatalf("[ERROR]: input file is already provided as `%s`.\n", inputFilePath)
			}

			inputFilePath = flag
		}
	}

	if inputFilePath == "" {
		usage(os.Stderr, program)
		log.Fatalf("[ERROR]: input was not provided\n")
	}

	vm := coppervm.Coppervm{}
	if _, err := vm.LoadProgramFromFile(inputFilePath); err != nil {
		log.Fatalf("[ERROR]: %s", err)
	}

	// Dump memory to stdout
	if printMemory {
		vm.DumpMemory()
		println()
	}

	// Dump program to stdout
	fmt.Fprintf(os.Stdout, "Entry point: %d\n", vm.Ip)
	for i := 0; i < len(vm.Program); i++ {
		inst := vm.Program[i]
		if printLineNbr {
			fmt.Fprintf(os.Stdout, "%d: ", i)
		}
		fmt.Fprintf(os.Stdout, "%s", inst)
		fmt.Fprintf(os.Stdout, "\n")
	}
}
