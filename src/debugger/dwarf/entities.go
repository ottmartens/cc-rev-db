package dwarf

import (
	"bytes"
	"debug/dwarf"
	"fmt"

	"github.com/go-delve/delve/pkg/dwarf/op"
)

type Module struct {
	name         string         // name of the module
	startAddress uint64         // the start of the address range in the module
	endAddress   uint64         // the end of the address range in the module
	entries      []Entry        // entries in this module
	files        map[int]string // source files of this module
	functions    []*Function    // functions declared in this module
	Variables    []*Variable    // variables declared in this module
}

type typeMap map[dwarf.Offset]*BaseType

type Entry struct {
	// Program-counter value of a machine instruction
	// generated by the compiler. This entry applies to
	// each instruction from pc to just before the pc of the next entry.
	Address uint64

	// key of the file in files map
	file int
	line int
	col  int

	// Address is one (of possibly many) PCs where execution should be
	// suspended for a breakpoint on exit from this function.
	prologueEnd bool

	// Address is one (of possibly many) PCs where execution should be
	// suspended for a breakpoint on exit from this function.
	epilogueBegin bool

	// Address is a recommended breakpoint location, such as the
	// beginning of a line, statement, or a distinct subpart of a statement.
	isStmt bool
}

type Function struct {
	name       string       // function name
	file       int          // file the function is declared at
	line       int64        // line nr
	col        int64        // col nr
	lowPC      uint64       // first PC address for the function
	highPC     uint64       // last PC address for the function
	parameters []*parameter // function parameters
}

type parameter struct {
	name                 string
	baseType             *BaseType            // type of the variable
	locationInstructions locationInstructions //
}

type BaseType struct {
	name     string
	byteSize int64
	encoding int64
}

type Variable struct {
	name                 string               // variable name
	baseType             *BaseType            // type of the variable
	locationInstructions locationInstructions // raw dwarf location instructions
	function             *Function            // the function where variable is declared (might be nil)
}

type MPIData struct {
	Functions []*Function // the debug info of wrapped mpi functions
	file      string      // file for mpi function wrappers
}

type locationInstructions []byte

func (m Module) String() string {
	functionString := ""
	for _, fn := range m.functions {
		functionString = fmt.Sprintf("%s\n%s", functionString, fn)
	}

	return fmt.Sprintf("{\nname:%s\nstart:%#x\nend:%#x\nfiles: %v\nfunctions: %v\n}", m.name, m.startAddress, m.endAddress, m.files, functionString)
}

func (fn Function) String() string {
	return fmt.Sprintf("{name:%s start:%#x end:%#x params:%v }", fn.name, fn.lowPC, fn.highPC, fn.parameters)
}

func (fn *Function) Name() string {
	if fn == nil {
		return "{nil}"
	}
	return fn.name
}

func (e Entry) String() string {
	return fmt.Sprintf("entry{address: %#x, file:%d, line: %d, col: %d, isStmt: %v}", e.Address, e.file, e.line, e.col, e.isStmt)
}

func (p parameter) String() string {
	return fmt.Sprintf("%s (%s)", p.name, p.baseType.name)
}

func (dMap typeMap) String() string {
	typesString := ""
	for offset, dType := range dMap {
		typesString = fmt.Sprintf("%soffset %#x - %v\n", typesString, offset, dType)
	}
	return fmt.Sprintf("typeMap:{\n%s}", typesString)
}

func (v *Variable) String() string {
	return fmt.Sprintf("{name:%v, type: %v, location: %v}", v.name, v.baseType.name, v.locationInstructions)
}

func (v *Variable) DecodeLocation(dRegisters op.DwarfRegisters) (address uint64, pieces []op.Piece, err error) {
	return v.locationInstructions.decode(dRegisters)
}

func (v *Variable) ByteSize() int64 {
	return v.baseType.byteSize
}

func (li locationInstructions) String() string {
	buf := new(bytes.Buffer)
	op.PrettyPrint(buf, []byte(li))
	return buf.String()
}

func (li locationInstructions) decode(dRegisters op.DwarfRegisters) (address uint64, pieces []op.Piece, err error) {
	addr, pieces, err := op.ExecuteStackProgram(dRegisters, li, ptrSize(), nil)
	return uint64(addr), pieces, err
}
