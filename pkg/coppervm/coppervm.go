package coppervm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

const (
	CoppervmDebug          bool  = false
	CoppervmStackCapacity  int64 = 1024
	CoppervmMemoryCapacity int64 = 1024
)

type InstAddr uint64

type Coppervm struct {
	// VM Stack
	Stack     [CoppervmStackCapacity]Word
	StackSize int64

	// VM Program
	Program []InstDef
	Ip      InstAddr

	// VM Memory
	Memory [CoppervmMemoryCapacity]byte

	// Opened File Descriptors
	FDs []*os.File

	// Is the VM halted?
	Halt     bool
	ExitCode int
}

// Load program's binary to vm from file.
func (vm *Coppervm) LoadProgramFromFile(filePath string) {
	content, fileErr := ioutil.ReadFile(filePath)
	if fileErr != nil {
		log.Fatalf("[ERROR]: Error reading file '%s': %s",
			filePath,
			fileErr)
	}

	var meta CoppervmFileMeta
	if err := json.Unmarshal(content, &meta); err != nil {
		log.Fatalf("[ERROR]: Error reading content of file '%s': %s",
			filePath,
			err)
	}

	// Init program
	vm.Halt = false
	vm.Ip = InstAddr(meta.Entry)
	vm.Program = meta.Program

	// Init memory
	if len(meta.Memory) > int(CoppervmMemoryCapacity) {
		log.Fatalf("[ERROR]: Memory exceed the maximum memory capacity!")
	}
	for i := 0; i < len(meta.Memory); i++ {
		vm.Memory[i] = meta.Memory[i]
	}

	// Append Stdin, Stdout, Stderr to open file descriptors
	vm.FDs = append(vm.FDs, os.Stdin)
	vm.FDs = append(vm.FDs, os.Stdout)
	vm.FDs = append(vm.FDs, os.Stderr)
}

// Executes all the program of the vm.
// Return a CoppervmError if something went wrong or ErrorOk.
func (vm *Coppervm) ExecuteProgram(limit int) *CoppervmError {
	for limit != 0 && !vm.Halt {
		if err := vm.ExecuteInstruction(); err.Kind != ErrorKindOk {
			return err
		}
		limit--
	}
	return ErrorOk(vm)
}

// Executes a single instruction of the program where the
// current ip points and then increments the ip.
// Return a CoppervmError if something went wrong or ErrorOk.
func (vm *Coppervm) ExecuteInstruction() *CoppervmError {
	if vm.Ip >= InstAddr(len(vm.Program)) {
		return ErrorIllegalInstAccess(vm)
	}

	currentInst := vm.Program[vm.Ip]
	switch currentInst.Kind {
	// Basic instructions
	case InstNoop:
		vm.Ip++
	case InstPush:
		if err := vm.pushStack(currentInst.Operand); err.Kind != ErrorKindOk {
			return err
		}
		vm.Ip++
	case InstSwap:
		if currentInst.Operand.AsI64 >= vm.StackSize {
			return ErrorStackUnderflow(vm)
		}
		a := vm.StackSize - 1
		b := vm.StackSize - 1 - currentInst.Operand.AsI64
		tmp := vm.Stack[a]
		vm.Stack[a] = vm.Stack[b]
		vm.Stack[b] = tmp
		vm.Ip++
	case InstDup:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		newVal := vm.Stack[vm.StackSize-1]
		if err := vm.pushStack(newVal); err.Kind != ErrorKindOk {
			return err
		}
		vm.Ip++
	case InstDrop:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		vm.StackSize--
		vm.Ip++
	case InstHalt:
		vm.haltVm(0)
	// Integer arithmetics
	case InstAddInt:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = AddWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstSubInt:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = SubWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstMulInt:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = MulWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstMulIntSigned:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = MulWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstDivInt:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsU64 == 0 {
			return ErrorDivideByZero(vm)
		}
		vm.Stack[vm.StackSize-2] = DivWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstDivIntSigned:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 == 0 {
			return ErrorDivideByZero(vm)
		}
		vm.Stack[vm.StackSize-2] = DivWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstModInt:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsU64 == 0 {
			return ErrorDivideByZero(vm)
		}
		vm.Stack[vm.StackSize-2] = ModWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstModIntSigned:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 == 0 {
			return ErrorDivideByZero(vm)
		}
		vm.Stack[vm.StackSize-2] = ModWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	// Floating point arithmetics
	case InstAddFloat:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = AddWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstSubFloat:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = SubWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstMulFloat:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		vm.Stack[vm.StackSize-2] = MulWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstDivFloat:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsF64 == 0 {
			return ErrorDivideByZero(vm)
		}
		vm.Stack[vm.StackSize-2] = DivWord(vm.Stack[vm.StackSize-2], vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	// Flow control
	case InstCmp:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		a := vm.Stack[vm.StackSize-2]
		b := vm.Stack[vm.StackSize-1]
		var res Word
		if a.AsI64 == b.AsI64 {
			res = WordI64(0)
		} else if a.AsI64 > b.AsI64 {
			res = WordI64(1)
		} else {
			res = WordI64(-1)
		}
		vm.Stack[vm.StackSize-2] = res
		vm.StackSize--
		vm.Ip++
	case InstJmp:
		vm.Ip = InstAddr(currentInst.Operand.AsI64)
	case InstJmpZero:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 == 0 {
			vm.Ip = InstAddr(currentInst.Operand.AsI64)
		} else {
			vm.Ip++
		}
		vm.StackSize--
	case InstJmpNotZero:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 != 0 {
			vm.Ip = InstAddr(currentInst.Operand.AsI64)
		} else {
			vm.Ip++
		}
		vm.StackSize--
	case InstJmpGreater:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 > 0 {
			vm.Ip = InstAddr(currentInst.Operand.AsI64)
		} else {
			vm.Ip++
		}
		vm.StackSize--
	case InstJmpLess:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 < 0 {
			vm.Ip = InstAddr(currentInst.Operand.AsI64)
		} else {
			vm.Ip++
		}
		vm.StackSize--
	case InstJmpGreaterEqual:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 >= 0 {
			vm.Ip = InstAddr(currentInst.Operand.AsI64)
		} else {
			vm.Ip++
		}
		vm.StackSize--
	case InstJmpLessEqual:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		if vm.Stack[vm.StackSize-1].AsI64 <= 0 {
			vm.Ip = InstAddr(currentInst.Operand.AsI64)
		} else {
			vm.Ip++
		}
		vm.StackSize--
	// Functions
	case InstFunCall:
		if err := vm.pushStack(WordU64(uint64(vm.Ip + 1))); err.Kind != ErrorKindOk {
			return err
		}
		vm.Ip = InstAddr(currentInst.Operand.AsU64)
	case InstFunReturn:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		retAdds := vm.Stack[vm.StackSize-1]
		vm.StackSize--
		vm.Ip = InstAddr(retAdds.AsU64)
	// Memory Access
	case InstMemRead:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		addr := vm.Stack[vm.StackSize-1].AsU64
		if addr >= uint64(CoppervmMemoryCapacity) {
			return ErrorIllegalMemoryAccess(vm)
		}
		vm.Stack[vm.StackSize-1] = WordU64(uint64(vm.Memory[addr]))
		vm.Ip++
	case InstMemWrite:
		if vm.StackSize < 2 {
			return ErrorStackUnderflow(vm)
		}
		addr := vm.Stack[vm.StackSize-1].AsU64
		if addr >= uint64(CoppervmMemoryCapacity) {
			return ErrorIllegalMemoryAccess(vm)
		}
		vm.Memory[addr] = byte(vm.Stack[vm.StackSize-2].AsU64)
		vm.StackSize -= 2
		vm.Ip++
	// Syscall
	case InstSyscall:
		sysCall := SysCall(currentInst.Operand.AsU64)
		switch sysCall {
		case SysCallRead:
			if vm.StackSize < 3 {
				return ErrorStackUnderflow(vm)
			}
			// Get count and start
			count := vm.Stack[vm.StackSize-1].AsU64
			bufStart := vm.Stack[vm.StackSize-2].AsU64
			if bufStart > uint64(CoppervmMemoryCapacity) {
				return ErrorIllegalMemoryAccess(vm)
			}

			// Get file descriptor
			fd := vm.Stack[vm.StackSize-3].AsU64
			if fd >= uint64(len(vm.FDs)) {
				vm.Stack[vm.StackSize-3] = WordI64(-1)
			} else {
				// Read form file
				file := vm.FDs[fd]
				buf := make([]byte, count)
				readBytesCount, err := file.Read(buf)
				if err != nil {
					vm.Stack[vm.StackSize-3] = WordI64(-1)
				} else {
					for i := bufStart; i < bufStart+uint64(readBytesCount); i++ {
						vm.Memory[i] = buf[i-bufStart]
					}
					vm.Stack[vm.StackSize-3] = WordU64(uint64(readBytesCount))
				}
			}
			vm.StackSize -= 2
			vm.Ip++
		case SysCallWrite:
			if vm.StackSize < 3 {
				return ErrorStackUnderflow(vm)
			}
			// Get count and start
			count := vm.Stack[vm.StackSize-1].AsU64
			bufStart := vm.Stack[vm.StackSize-2].AsU64
			if bufStart > uint64(CoppervmMemoryCapacity) {
				return ErrorIllegalMemoryAccess(vm)
			}
			buf := vm.Memory[bufStart : bufStart+count]

			// Get file descriptor
			fd := vm.Stack[vm.StackSize-3].AsU64
			if fd >= uint64(len(vm.FDs)) {
				vm.Stack[vm.StackSize-3] = WordI64(-1)
			} else {
				// Write to file
				file := vm.FDs[fd]
				writtenBytesCount, err := file.Write(buf)
				if err != nil {
					vm.Stack[vm.StackSize-3] = WordI64(-1)
				} else {
					vm.Stack[vm.StackSize-3] = WordU64(uint64(writtenBytesCount))
				}
			}
			vm.StackSize -= 2
			vm.Ip++
		case SysCallOpen:
			if vm.StackSize < 1 {
				return ErrorStackUnderflow(vm)
			}
			// Get file name form memory
			bufStart := vm.Stack[vm.StackSize-1].AsU64
			if int64(bufStart) > CoppervmMemoryCapacity {
				return ErrorIllegalMemoryAccess(vm)
			}
			var fileNameBytes []byte
			for i := int(bufStart); i < len(vm.Memory); i++ {
				if vm.Memory[i] != 0 {
					fileNameBytes = append(fileNameBytes, vm.Memory[i])
				} else {
					break
				}
			}
			// Open the file
			// TODO: Files are opened only in O_RDWR mode
			fd, err := os.OpenFile(string(fileNameBytes), os.O_RDWR, os.ModePerm)
			if err != nil {
				vm.Stack[vm.StackSize-1] = WordI64(-1)
			} else {
				vm.FDs = append(vm.FDs, fd)
				vm.Stack[vm.StackSize-1] = WordI64(int64(len(vm.FDs) - 1))
			}
			vm.Ip++
		case SysCallClose:
			if vm.StackSize < 1 {
				return ErrorStackUnderflow(vm)
			}
			// Get file descriptor
			fd := vm.Stack[vm.StackSize-1].AsU64
			if fd >= uint64(len(vm.FDs)) {
				vm.Stack[vm.StackSize-1] = WordI64(-1)
			} else {
				// Close the file
				file := vm.FDs[fd]
				err := file.Close()
				if err != nil {
					vm.Stack[vm.StackSize-1] = WordI64(-1)
				} else {
					vm.FDs = append(vm.FDs[:fd], vm.FDs[fd+1:]...)
					vm.Stack[vm.StackSize-1] = WordU64(0)
				}
			}
			vm.Ip++
		case SysCallSeek:
			if vm.StackSize < 3 {
				return ErrorStackUnderflow(vm)
			}
			// Get offset and whence
			whence := vm.Stack[vm.StackSize-1].AsI64
			offset := vm.Stack[vm.StackSize-2].AsI64
			// Get file descriptor
			fd := vm.Stack[vm.StackSize-3].AsU64
			if fd >= uint64(len(vm.FDs)) {
				vm.Stack[vm.StackSize-3] = WordI64(-1)
			} else {
				// Seek the file
				file := vm.FDs[fd]
				newPosition, err := file.Seek(offset, int(whence))
				if err != nil {
					vm.Stack[vm.StackSize-3] = WordI64(-1)
				} else {
					vm.Stack[vm.StackSize-3] = WordI64(newPosition)
				}
			}
			vm.StackSize -= 2
			vm.Ip++
		case SysCallExit:
			if vm.StackSize < 1 {
				return ErrorStackUnderflow(vm)
			}
			statusCode := vm.Stack[vm.StackSize-1]
			vm.haltVm(int(statusCode.AsI64))
			vm.StackSize--
		default:
			log.Fatalf("Unknown system call %d", sysCall)
		}
	// Debug print
	case InstPrint:
		if vm.StackSize < 1 {
			return ErrorStackUnderflow(vm)
		}
		fmt.Printf("%s\n", vm.Stack[vm.StackSize-1])
		vm.StackSize--
		vm.Ip++
	case InstCount:
		fallthrough
	default:
		log.Fatalf("Invalid instruction %s", currentInst.Name)
	}

	// Print stack on debug
	if CoppervmDebug {
		vm.dumpStack()
	}

	return ErrorOk(vm)
}

// Push a Word to the stack.
// Return a ErrorStackOverflow if the stack overflows, or
// ErrorOk otherwise.
func (vm *Coppervm) pushStack(w Word) *CoppervmError {
	if vm.StackSize >= CoppervmStackCapacity {
		return ErrorStackOverflow(vm)
	}
	vm.Stack[vm.StackSize] = w
	vm.StackSize++
	return ErrorOk(vm)
}

// Set the virtual machine in an halt state.
func (vm *Coppervm) haltVm(code int) {
	// Set halt flag to true
	vm.Halt = true
	// Set status code to code
	vm.ExitCode = code
	// Close all open files
	for _, f := range vm.FDs {
		f.Close()
	}
}

// Prints the stack content to standard output.
func (vm *Coppervm) dumpStack() {
	fmt.Printf("Stack:\n")
	if vm.StackSize > 0 {
		for i := int64(0); i < vm.StackSize; i++ {
			fmt.Printf("  %s\n", vm.Stack[i])
		}
	} else {
		fmt.Printf("  [empty]\n")
	}
}
