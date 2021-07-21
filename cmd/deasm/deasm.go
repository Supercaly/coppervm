package main

import (
	"fmt"
	"io"
	"log"
	"os"

	au "coppervm.com/coppervm/internal"
	"coppervm.com/coppervm/pkg/coppervm"
)

func usage(stream io.Writer, program string) {
	fmt.Fprintf(stream, "Usage: %s [OPTIONS] <input.vm>\n", program)
	fmt.Fprintf(stream, "[OPTIONS]: \n")
	fmt.Fprintf(stream, "    -h     Print this help message.\n")
}

func main() {
	args := os.Args
	var program string
	program, args = au.Shift(args)
	var inputFilePath string

	for len(args) > 0 {
		var flag string
		flag, args = au.Shift(args)

		if flag == "-h" {
			usage(os.Stdout, program)
			os.Exit(0)
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
	vm.LoadProgramFromFile(inputFilePath)

	// Dump program to stdout
	fmt.Fprintf(os.Stdout, "Entry point: %d\n", vm.Ip)
	for i := 0; i < len(vm.Program); i++ {
		inst := vm.Program[i]
		fmt.Fprintf(os.Stdout, "%s ", inst.Name)
		if inst.HasOperand {
			fmt.Fprintf(os.Stdout, "%d", inst.Operand)
		}
		fmt.Fprintf(os.Stdout, "\n")
	}
}