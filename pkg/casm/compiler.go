package casm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Supercaly/coppervm/internal"
	"github.com/Supercaly/coppervm/pkg/coppervm"
)

const (
	CasmMaxIncludeLevel int    = 10
	CasmFileExtention   string = ".casm"
)

type Casm struct {
	InputFile  string
	OutputFile string

	AddDebugSymbols bool

	Bindings         []Binding
	DeferredOperands []DeferredOperand

	Program []coppervm.InstDef

	HasEntry          bool
	Entry             int
	EntryLocation     FileLocation
	DeferredEntryName string

	Memory []byte

	IncludeLevel int
	IncludePaths []string

	StringLengths map[int]int
}

// Save a copper vm program to binary file.
func (casm *Casm) SaveProgramToFile() error {
	var dbSymbols coppervm.DebugSymbols
	// Append debug symbols
	if casm.AddDebugSymbols {
		for _, b := range casm.Bindings {
			if b.IsLabel {
				dbSymbols = append(dbSymbols, coppervm.DebugSymbol{
					Name:    b.Name,
					Address: coppervm.InstAddr(b.Value.AsNumLitInt),
				})
			}
		}
	}

	meta := coppervm.FileMeta(casm.Entry, casm.Program, casm.Memory, dbSymbols)
	metaJson, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("error writing program to file %s", err)
	}

	if filepath.Ext(casm.OutputFile) != coppervm.CoppervmFileExtention {
		return fmt.Errorf("file '%s' is not a valid %s file", casm.OutputFile, coppervm.CoppervmFileExtention)
	}
	fileErr := ioutil.WriteFile(casm.OutputFile, []byte(metaJson), os.ModePerm)
	if fileErr != nil {
		return fmt.Errorf("error saving file '%s': %s", casm.OutputFile, fileErr)
	}
	fmt.Printf("[INFO]: Program saved to '%s'\n", casm.OutputFile)

	return nil
}

// Translate a copper assembly file to copper vm's binary.
// Given a file path this function will read it and generate
// the correct program in-memory.
// Use TranslateSource is you already have a source string.
// Use SaveProgramToFile to save the program to binary file.
func (casm *Casm) TranslateSourceFile(filePath string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	source := casm.readSourceFile(filePath)

	internal.DebugPrint("[INFO]: Building program '%s'\n", filePath)
	// Linize the source
	lines, err := Linize(source, filePath)
	if err != nil {
		panic(err)
	}

	// Create intermediate representation
	irs := casm.translateIR(lines)

	// First pass
	casm.firstPass(irs)

	// Second pass
	casm.secondPass()

	internal.DebugPrint("[INFO]: Built program '%s'\n", filePath)
	return err
}

// Reads a source string from given file path.
func (casm *Casm) readSourceFile(filePath string) string {
	if filepath.Ext(filePath) != CasmFileExtention {
		panic(fmt.Sprintf("file '%s' is not a valid %s file", filePath, CasmFileExtention))
	}
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic(fmt.Sprintf("error reading file '%s': %s", filePath, err))
	}
	return string(bytes)
}

// Convert lines to intermediate representation.
func (casm *Casm) translateIR(lines []Line) (out []IR) {
	for _, line := range lines {
		switch line.Kind {
		case LineKindLabel:
			if line.AsLabel.Name == "" {
				panic(fmt.Sprintf("%s: empty labels are not supported", line.Location))
			}

			out = append(out, IR{
				Kind:     IRKindLabel,
				AsLabel:  LabelIR{line.AsLabel.Name},
				Location: line.Location,
			})
		case LineKindInstruction:
			exist, instDef := coppervm.GetInstDefByName(line.AsInstruction.Name)
			if !exist {
				panic(fmt.Sprintf("%s: unknown instruction '%s'",
					line.Location,
					line.AsInstruction.Name))
			}

			var operand Expression
			if instDef.HasOperand {
				var err error
				operand, err = ParseExprFromString(line.AsInstruction.Operand)
				if err != nil {
					panic(fmt.Sprintf("%s: %s", line.Location, err))
				}
			}
			out = append(out, IR{
				Kind: IRKindInstruction,
				AsInstruction: InstructionIR{
					Name:       instDef.Name,
					Operand:    operand,
					HasOperand: instDef.HasOperand,
				},
				Location: line.Location,
			})
		case LineKindDirective:
			switch line.AsDirective.Name {
			case "entry":
				out = append(out, IR{
					Kind: IRKindEntry,
					AsEntry: EntryIR{
						Name: line.AsDirective.Block,
					},
					Location: line.Location,
				})
			case "const":
				name, block := internal.SplitByDelim(line.AsDirective.Block, ' ')
				name = strings.TrimSpace(name)
				block = strings.TrimSpace(block)
				expr, err := ParseExprFromString(block)
				if err != nil {
					panic(fmt.Sprintf("%s: %s", line.Location, err))
				}

				out = append(out, IR{
					Kind: IRKindConst,
					AsConst: ConstIR{
						Name:  name,
						Value: expr,
					},
					Location: line.Location,
				})
			case "memory":
				name, block := internal.SplitByDelim(line.AsDirective.Block, ' ')
				name = strings.TrimSpace(name)
				block = strings.TrimSpace(block)
				expr, err := ParseExprFromString(block)
				if err != nil {
					panic(fmt.Sprintf("%s: %s", line.Location, err))
				}

				out = append(out, IR{
					Kind: IRKindMemory,
					AsMemory: MemoryIR{
						Name:  name,
						Value: expr,
					},
					Location: line.Location,
				})
			case "include":
				out = append(out, casm.translateInclude(line.AsDirective, line.Location)...)
			default:
				panic(fmt.Sprintf("%s: unknown directive '%s'", line.Location, line.AsDirective.Name))
			}
		}
	}

	return out
}

// Do the first pass in the parsing process.
func (casm *Casm) firstPass(irs []IR) {
	for _, ir := range irs {
		switch ir.Kind {
		case IRKindLabel:
			casm.bindLabel(ir.AsLabel, len(casm.Program), ir.Location)
		case IRKindInstruction:
			inst := ir.AsInstruction
			_, instDef := coppervm.GetInstDefByName(inst.Name)

			if instDef.HasOperand {
				if inst.Operand.Kind == ExpressionKindBinding {
					casm.DeferredOperands = append(casm.DeferredOperands,
						DeferredOperand{
							Name:     inst.Operand.AsBinding,
							Address:  len(casm.Program),
							Location: ir.Location,
						})
				} else {
					instDef.Operand = casm.evaluateExpression(inst.Operand, ir.Location).Word
				}
			}
			casm.Program = append(casm.Program, instDef)
		case IRKindEntry:
			casm.bindEntry(ir.AsEntry, ir.Location)
		case IRKindConst:
			casm.bindConst(ir.AsConst, ir.Location)
		case IRKindMemory:
			casm.bindMemory(ir.AsMemory, ir.Location)
		}
	}
}

// Do the second pass in the parsing process.
func (casm *Casm) secondPass() {
	for _, deferredOp := range casm.DeferredOperands {
		exist, binding := casm.getBindingByName(deferredOp.Name)
		if !exist {
			panic(fmt.Sprintf("%s: unknown binding '%s'",
				deferredOp.Location,
				deferredOp.Name))
		}
		casm.Program[deferredOp.Address].Operand = casm.evaluateBinding(binding,
			deferredOp.Location).Word
	}

	// Print all the bindings
	if internal.DebugPrintEnabled() {
		internal.DebugPrint("[INFO]: bindings:\n")
		for _, b := range casm.Bindings {
			internal.DebugPrint("  %s\n", b)
		}
	}

	// Resolve entry point
	if casm.HasEntry && casm.DeferredEntryName != "" {
		exist, binding := casm.getBindingByName(casm.DeferredEntryName)
		if !exist {
			panic(fmt.Sprintf("%s: unknown binding '%s'",
				casm.EntryLocation,
				casm.DeferredEntryName))
		}

		if binding.Value.Kind != ExpressionKindNumLitInt {
			panic(fmt.Sprintf("%s: only label names can be set as entry point",
				casm.EntryLocation))
		}
		entry := casm.evaluateBinding(binding, casm.EntryLocation).Word
		casm.Entry = int(entry.AsI64)
	}

	// Check if halt instruction exist only in the main program
	// skip this when translating includes
	if casm.IncludeLevel == 0 {
		hasHalt := false
		for _, inst := range casm.Program {
			if inst.Kind == coppervm.InstHalt {
				hasHalt = true
			}
		}
		if !hasHalt {
			fmt.Printf("[WARN]: no 'halt' instruction found in the program! This program could not work as intended.\n")
		}
	}
}

// Returns a binding by its name.
// If the binding exist the first return parameter will be true,
// otherwise it'll be null.
func (casm *Casm) getBindingByName(name string) (bool, Binding) {
	for _, b := range casm.Bindings {
		if b.Name == name {
			return true, b
		}
	}
	return false, Binding{}
}

// Returns the index of a binding by it's name.
// If the binding doesn't exist -1 is returned.
func (casm *Casm) getBindingIndexByName(name string) int {
	for idx, b := range casm.Bindings {
		if b.Name == name {
			return idx
		}
	}
	return -1
}

// Binds a label.
func (casm *Casm) bindLabel(label LabelIR, address int, location FileLocation) {
	exist, binding := casm.getBindingByName(label.Name)
	if exist {
		panic(fmt.Sprintf("%s: label name '%s' is already bound at location '%s'",
			location,
			label.Name,
			binding.Location))
	}

	casm.Bindings = append(casm.Bindings, Binding{
		Status:        BindingEvaluated,
		Name:          label.Name,
		EvaluatedWord: coppervm.WordU64(uint64(address)),
		Location:      location,
		IsLabel:       true,
	})
}

// Binds a constant.
func (casm *Casm) bindConst(constIR ConstIR, location FileLocation) {
	exist, binding := casm.getBindingByName(constIR.Name)
	if exist {
		panic(fmt.Sprintf("%s: constant name '%s' is already bound at location '%s'",
			location,
			constIR.Name,
			binding.Location))
	}

	newBinding := Binding{
		Status:   BindingUnevaluated,
		Name:     constIR.Name,
		Value:    constIR.Value,
		Location: location,
		IsLabel:  false,
	}

	// If it's a const string push it in memory and bind his base address
	if constIR.Value.Kind == ExpressionKindStringLit {
		baseAddr := casm.pushStringToMemory(constIR.Value.AsStringLit)
		newBinding.EvaluatedWord = coppervm.WordU64(uint64(baseAddr))
		newBinding.Status = BindingEvaluated
	}

	casm.Bindings = append(casm.Bindings, newBinding)
}

// Binds an entry point.
func (casm *Casm) bindEntry(entry EntryIR, location FileLocation) {
	if casm.HasEntry {
		panic(fmt.Sprintf("%s: entry point is already set to '%s'",
			location,
			casm.EntryLocation))
	}

	casm.DeferredEntryName = entry.Name
	casm.HasEntry = true
	casm.EntryLocation = location
}

// Binds a memory definition.
func (casm *Casm) bindMemory(memory MemoryIR, location FileLocation) {
	exist, binding := casm.getBindingByName(memory.Name)
	if exist {
		panic(fmt.Sprintf("%s: memory name '%s' is already bound at location '%s'",
			location,
			memory.Name,
			binding.Location))
	}

	if memory.Value.Kind != ExpressionKindByteList {
		panic(fmt.Sprintf("%s: expected '%s' but got '%s'",
			location, ExpressionKindByteList, memory.Value.Kind))
	}
	memAddr := len(casm.Memory)
	casm.Memory = append(casm.Memory, memory.Value.AsByteList...)

	casm.Bindings = append(casm.Bindings, Binding{
		Status:        BindingEvaluated,
		Name:          memory.Name,
		EvaluatedWord: coppervm.WordU64(uint64(memAddr)),
		Location:      location,
		IsLabel:       false,
	})
}

// Translate include directive.
func (casm *Casm) translateInclude(directive DirectiveLine, location FileLocation) (out []IR) {
	exist, resolvedPath := casm.resolveIncludePath(directive.Block)
	if !exist {
		panic(fmt.Sprintf("%s: cannot resolve include file '%s'", location, directive.Block))
	}

	if casm.IncludeLevel >= CasmMaxIncludeLevel {
		panic("maximum include level reached")
	}

	// Generate IR from included file
	casm.IncludeLevel++
	includeSource := casm.readSourceFile(resolvedPath)
	lines, err := Linize(includeSource, resolvedPath)
	if err != nil {
		panic(err)
	}
	out = casm.translateIR(lines)
	casm.IncludeLevel--

	return out
}

// Resolve an include path from the list of includes.
func (casm *Casm) resolveIncludePath(path string) (exist bool, resolved string) {
	// Check the include paths
	for _, includePath := range casm.IncludePaths {
		resolved = filepath.Join(includePath, path)
		internal.DebugPrint("[INFO]: search for '%s' in %s\n", path, includePath)
		_, err := os.Stat(resolved)
		if err == nil {
			return true, resolved
		}
	}
	return false, ""
}

// Represent the result of an expression evaluation.
type EvalResult struct {
	Word coppervm.Word
	Type ExpressionKind
}

// Evaluate a binding to extract am eval result.
func (casm *Casm) evaluateBinding(binding Binding, location FileLocation) (ret EvalResult) {
	switch binding.Status {
	case BindingUnevaluated:
		idx := casm.getBindingIndexByName(binding.Name)
		if idx == -1 {
			panic(fmt.Sprintf("%s: cannot find index binding %s", location, binding.Name))
		}
		casm.Bindings[idx].Status = BindingEvaluating
		ret = casm.evaluateExpression(binding.Value, location)
		casm.Bindings[idx].Status = BindingEvaluated
		casm.Bindings[idx].EvaluatedWord = ret.Word
	case BindingEvaluating:
		panic(fmt.Sprintf("%s: cycling binding definition detected", location))
	case BindingEvaluated:
		ret = EvalResult{
			binding.EvaluatedWord,
			binding.Value.Kind,
		}
	}
	internal.DebugPrint("[INFO]: evaluated binding with result %s\n", ret)
	return ret
}

// Evaluate an expression to extract an eval result.
func (casm *Casm) evaluateExpression(expr Expression, location FileLocation) (ret EvalResult) {
	switch expr.Kind {
	case ExpressionKindBinding:
		exist, binding := casm.getBindingByName(expr.AsBinding)
		if !exist {
			panic(fmt.Sprintf("%s: cannot find binding '%s'", location, expr.AsBinding))
		}
		ret = casm.evaluateBinding(binding, location)
	case ExpressionKindNumLitInt:
		ret = EvalResult{
			coppervm.WordI64(expr.AsNumLitInt),
			ExpressionKindNumLitInt,
		}
	case ExpressionKindNumLitFloat:
		ret = EvalResult{
			coppervm.WordF64(expr.AsNumLitFloat),
			ExpressionKindNumLitFloat,
		}
	case ExpressionKindStringLit:
		strBase := casm.pushStringToMemory(expr.AsStringLit)
		ret = EvalResult{
			coppervm.WordU64(uint64(strBase)),
			ExpressionKindStringLit,
		}
	case ExpressionKindBinaryOp:
		ret = casm.evaluateBinaryOp(expr, location)
	case ExpressionKindByteList:
		panic(fmt.Sprintf("%s: cannot use byte lists as operands, only supported use is in memory directives", location))
	}
	internal.DebugPrint("[INFO]: evaluated expression with result %s\n", ret)
	return ret
}

// Map of types of binary operation sides to
// the result type.
// The unsupported operations between types are
// marked as -1 following this table:
//
//[
//   // i  f  s  b  o
//     [i, f, -, -, -], //i
//     [f, f, -, -, -], //f
//     [-, -, s, -, -], //s
//     [-, -, -, -, -], //o
//     [-, -, -, -, -], //b
// ]
var binaryOpEvaluationMap = [6][6]ExpressionKind{
	{ExpressionKindNumLitInt, ExpressionKindNumLitFloat, -1, -1, -1, -1},
	{ExpressionKindNumLitFloat, ExpressionKindNumLitFloat, -1, -1, -1, -1},
	{-1, -1, ExpressionKindStringLit, -1, -1, -1},
	{-1, -1, -1, -1, -1, -1},
	{-1, -1, -1, -1, -1, -1},
	{-1, -1, -1, -1, -1, -1},
}

// Map an ExpressionKind to a TypeRepresentation.
var exprKindToTypeRepMap = map[ExpressionKind]coppervm.TypeRepresentation{
	ExpressionKindNumLitInt:   coppervm.TypeI64,
	ExpressionKindNumLitFloat: coppervm.TypeF64,
	ExpressionKindStringLit:   coppervm.TypeU64,
}

// Evaluate a binary op expression to extract an eval result.
func (casm *Casm) evaluateBinaryOp(binop Expression, location FileLocation) (result EvalResult) {
	lhs_result := casm.evaluateExpression(*binop.AsBinaryOp.Lhs, location)
	rhs_result := casm.evaluateExpression(*binop.AsBinaryOp.Rhs, location)

	resultType := binaryOpEvaluationMap[lhs_result.Type][rhs_result.Type]
	if resultType == -1 {
		panic(fmt.Sprintf("%s: unsupported binary operation between types '%s' and '%s'",
			location,
			lhs_result.Type,
			rhs_result.Type))
	}

	// At this point the only permitted operations are between
	// int-int int-float float-int float-float string-string
	// so we can reduce the next checks.

	if resultType == ExpressionKindStringLit {
		// The op is string-string
		if binop.AsBinaryOp.Kind != BinaryOpKindPlus {
			panic(fmt.Sprintf("%s: unsupported operations ['-', '*', '/', '%%'] between string literals",
				location))
		}
		leftStr := casm.getStringByAddress(int(lhs_result.Word.AsU64))
		rightStr := casm.getStringByAddress(int(rhs_result.Word.AsU64))
		result = EvalResult{
			coppervm.WordU64(uint64(casm.pushStringToMemory(leftStr + rightStr))),
			ExpressionKindStringLit,
		}
	} else {
		// The only ops at this point are int-float float-int.
		// int-int and float-float are removed because in Expression we precompute
		// the operations with same type
		resultTypeRep := exprKindToTypeRepMap[resultType]
		switch binop.AsBinaryOp.Kind {
		case BinaryOpKindPlus:
			result = EvalResult{coppervm.AddWord(lhs_result.Word, rhs_result.Word, resultTypeRep), resultType}
		case BinaryOpKindMinus:
			result = EvalResult{coppervm.SubWord(lhs_result.Word, rhs_result.Word, resultTypeRep), resultType}
		case BinaryOpKindTimes:
			result = EvalResult{coppervm.MulWord(lhs_result.Word, rhs_result.Word, resultTypeRep), resultType}
		case BinaryOpKindDivide:
			if rhs_result.Word.AsI64 == 0 || rhs_result.Word.AsF64 == 0.0 {
				panic(fmt.Sprintf("%s: divide by zero", location))
			}
			result = EvalResult{coppervm.DivWord(lhs_result.Word, rhs_result.Word, resultTypeRep), resultType}
		case BinaryOpKindModulo:
			// Since the only pos are int-float and float-int allways panic
			panic(fmt.Sprintf("%s: unsupported '%%' operation between floating point literals", location))
		}
	}
	return result
}

// Push a string to casm memory and return the base address.
func (casm *Casm) pushStringToMemory(str string) int {
	strBase := len(casm.Memory)
	byteStr := []byte(str)
	byteStr = append(byteStr, 0)
	casm.Memory = append(casm.Memory, byteStr...)

	if casm.StringLengths == nil {
		casm.StringLengths = make(map[int]int)
	}
	casm.StringLengths[strBase] = len(byteStr)
	return strBase
}

// Returns a string from memory at given address without
// null termination.
// If the string doesn't exist an empty string is returned.
func (casm *Casm) getStringByAddress(addr int) string {
	strLen := casm.StringLengths[addr]
	if strLen == 0 {
		return ""
	}
	strBytes := casm.Memory[addr : addr+strLen-1]
	return string(strBytes[:])
}
