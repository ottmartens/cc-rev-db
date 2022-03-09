package main

//lint:file-ignore U1000 ignore unused helpers

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"io"
	"reflect"

	"github.com/go-delve/delve/pkg/dwarf/op"
	"github.com/ottmartens/cc-rev-db/logger"
)

type dwarfData struct {
	modules []*dwarfModule
	types   dwarfTypeMap
	mpi     dwarfMPIData
}
type dwarfModule struct {
	name         string           // name of the module
	startAddress uint64           // the start of the address range in the module
	endAddress   uint64           // the end of the address range in the module
	entries      []dwarfEntry     // entries in this module
	files        map[int]string   // source files of this module
	functions    []*dwarfFunc     // functions declared in this module
	variables    []*dwarfVariable // variables declared in this module
}

type dwarfTypeMap map[dwarf.Offset]*dwarfBaseType
type dwarfEntry struct {
	// Program-counter value of a machine instruction
	// generated by the compiler. This entry applies to
	// each instruction from pc to just before the pc of the next entry.
	address uint64

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

func (m dwarfModule) String() string {
	functionString := ""
	for _, fn := range m.functions {
		functionString = fmt.Sprintf("%s\n%s", functionString, fn)
	}

	return fmt.Sprintf("{\nname:%s\nstart:%#x\nend:%#x\nfiles: %v\nfunctions: %v\n}", m.name, m.startAddress, m.endAddress, m.files, functionString)
}

func (fn dwarfFunc) String() string {
	return fmt.Sprintf("{name:%s start:%#x end:%#x params:%v }", fn.name, fn.lowPC, fn.highPC, fn.parameters)
}

func (fn *dwarfFunc) Name() string {
	if fn == nil {
		return "{nil}"
	}
	return fn.name
}

func (e dwarfEntry) String() string {
	return fmt.Sprintf("entry{address: %#x, file:%d, line: %d, col: %d, isStmt: %v}", e.address, e.file, e.line, e.col, e.isStmt)
}

func (p dwarfParameter) String() string {
	return fmt.Sprintf("%s (%s)", p.name, p.baseType.name)
}

func (dMap dwarfTypeMap) String() string {
	typesString := ""
	for offset, dType := range dMap {
		typesString = fmt.Sprintf("%soffset %#x - %v\n", typesString, offset, dType)
	}
	return fmt.Sprintf("typeMap:{\n%s}", typesString)
}

func (v *dwarfVariable) String() string {
	return fmt.Sprintf("{name:%v, type: %v, location: %v}", v.name, v.baseType.name, v.locationInstructions)
}

type dwarfFunc struct {
	name       string            // function name
	file       int               // file the function is declared at
	line       int64             // line nr
	col        int64             // col nr
	lowPC      uint64            // first PC address for the function
	highPC     uint64            // last PC address for the function
	parameters []*dwarfParameter // function parameters
}

type dwarfParameter struct {
	name                 string
	baseType             *dwarfBaseType            // type of the variable
	locationInstructions dwarfLocationInstructions //
}

type dwarfBaseType struct {
	name     string
	byteSize int64
	encoding int64
}

type dwarfVariable struct {
	name                 string                    // variable name
	baseType             *dwarfBaseType            // type of the variable
	locationInstructions dwarfLocationInstructions // raw dwarf location instructions
	function             *dwarfFunc                // the function where variable is declared (might be nil)
}

type dwarfMPIData struct {
	functions []*dwarfFunc // the debug info of wrapped mpi functions
	file      string       // file for mpi function wrappers
}

type dwarfLocationInstructions []byte

func (li dwarfLocationInstructions) String() string {
	buf := new(bytes.Buffer)
	op.PrettyPrint(buf, []byte(li))
	return buf.String()
}

func (li dwarfLocationInstructions) decode(dRegisters op.DwarfRegisters) (address uint64, pieces []op.Piece, err error) {
	addr, pieces, err := op.ExecuteStackProgram(dRegisters, li, ptrSize(), nil)
	return uint64(addr), pieces, err
}

func (m *dwarfModule) lookupFunc(functionName string) *dwarfFunc {
	for _, function := range m.functions {
		if function.name == functionName {
			return function
		}
	}
	return nil
}

func (d *dwarfData) lookupFunc(functionName string) (module *dwarfModule, function *dwarfFunc) {
	for _, module := range d.modules {
		if function := module.lookupFunc(functionName); function != nil {
			return module, function
		}
	}
	return nil, nil
}

func (d *dwarfData) lookupVariable(varName string) *dwarfVariable {
	for _, module := range d.modules {
		for _, variable := range module.variables {
			if variable.name == varName {
				return variable
			}
		}
	}

	return nil
}

func (d *dwarfData) getEntriesForFunction(functionName string) []dwarfEntry {
	entries := make([]dwarfEntry, 0)
	module, function := d.lookupFunc(functionName)

	for _, entry := range module.entries {
		if entry.address >= function.lowPC && entry.address < function.highPC {
			entries = append(entries, entry)
		}
	}

	return entries
}

func (d *dwarfData) lineToPC(file string, line int) (address uint64, err error) {

	for _, module := range d.modules {
		for _, moduleFile := range module.files {
			if moduleFile == file {
				for _, entry := range module.entries {
					if entry.line == line && module.files[entry.file] == file {
						if entry.isStmt {
							return entry.address, nil
						} else {
							logger.Info("non-stmt exists")
						}
					}
				}
			}
		}
	}

	return 0, fmt.Errorf("unable to find suitable instruction for line %d in file %s", line, file)
}

func (d *dwarfData) PCToLine(pc uint64) (line int, file string, function *dwarfFunc, err error) {
	for _, module := range d.modules {
		if pc >= module.startAddress && pc <= module.endAddress {
			for _, entry := range module.entries {
				if entry.address == pc {

					function := d.PCToFunc(pc)

					return entry.line, module.files[entry.file], function, nil

				}
			}
		}
	}
	return 0, "", nil, fmt.Errorf("unable to find instruction matching address %v", pc)
}

func (d *dwarfData) PCToFunc(pc uint64) *dwarfFunc {
	// logger.Info("pc to func %#x", pc)
	for _, module := range d.modules {
		for _, function := range module.functions {
			if pc >= function.lowPC && pc < function.highPC {
				return function
			}
			// logger.Info("func %v does not match", function)
		}
	}

	return nil
}

func getDwarfData(targetFile string) *dwarfData {

	data := &dwarfData{
		modules: make([]*dwarfModule, 0),
		types:   make(dwarfTypeMap),
	}
	var currentModule *dwarfModule
	var currentFunction *dwarfFunc

	elfFile, err := elf.Open(targetFile)
	if err != nil {
		panic(err)
	}
	dwarfRawData, err := elfFile.DWARF()
	if err != nil {
		panic(err)
	}

	reader := dwarfRawData.Reader()

	for {
		entry, err := reader.Next()

		if err == io.EOF || entry == nil {
			break
		}
		if err != nil {
			panic(err)
		}

		switch entry.Tag {

		// base type declaration
		case dwarf.TagBaseType:
			data.types[entry.Offset] = &dwarfBaseType{
				name:     entry.Val(dwarf.AttrName).(string),
				byteSize: entry.Val(dwarf.AttrByteSize).(int64),
				encoding: entry.Val(dwarf.AttrEncoding).(int64),
			}

		// entering a new module
		case dwarf.TagCompileUnit:
			currentModule = parseModule(entry, dwarfRawData)

			data.modules = append(data.modules, currentModule)

			currentFunction = nil

		// function declaration
		case dwarf.TagSubprogram:
			currentFunction = parseFunction(entry, dwarfRawData)

			currentModule.functions = append(currentModule.functions, currentFunction)

		case dwarf.TagFormalParameter:
			parameter := parseFunctionParameter(entry, data)

			currentFunction.parameters = append(currentFunction.parameters, parameter)

		// variable declaration
		case dwarf.TagVariable:
			baseType := data.types[entry.Val(dwarf.AttrType).(dwarf.Offset)]

			if baseType == nil {
				baseType = &dwarfBaseType{
					name: "unknown type",
				}
			}

			variable := &dwarfVariable{
				name:     entry.Val(dwarf.AttrName).(string),
				baseType: baseType,
				function: currentFunction,
			}

			locationInstructions := entry.Val(dwarf.AttrLocation)

			if reflect.TypeOf(locationInstructions) != nil {
				variable.locationInstructions = entry.Val(dwarf.AttrLocation).([]byte)
			}

			currentModule.variables = append(currentModule.variables, variable)

		default:
			// logger.Info("unhandled tag type: %v", entry.Tag)
		}

	}

	data.mpi = resolveMPIDebugInfo(data)

	return data
}

func parseFunctionParameter(entry *dwarf.Entry, data *dwarfData) *dwarfParameter {

	baseType := data.types[(entry.Val(dwarf.AttrType).(dwarf.Offset))]

	if baseType == nil {
		baseType = &dwarfBaseType{
			name: "unknown type",
		}
	}

	parameter := &dwarfParameter{
		name:                 entry.Val(dwarf.AttrName).(string),
		baseType:             baseType,
		locationInstructions: entry.Val(dwarf.AttrLocation).([]byte),
	}

	return parameter
}

func parseFunction(entry *dwarf.Entry, dwarfRawData *dwarf.Data) *dwarfFunc {
	function := dwarfFunc{}

	for _, field := range entry.Field {
		switch field.Attr {
		case dwarf.AttrName:
			function.name = field.Val.(string)
		case dwarf.AttrDeclFile:
			function.file = int(field.Val.(int64))
		case dwarf.AttrDeclLine:
			function.line = field.Val.(int64)
			// adjust for inserted line
			function.line--
		case dwarf.AttrDeclColumn:
			function.col = field.Val.(int64)
		case dwarf.AttrFrameBase:

			// fmt.Printf("frame base : %v, %v, %T, %x\n", field.Attr, field.Val, field.Val, field.Val)

			// buf := new(bytes.Buffer)
			// op.PrettyPrint(buf, field.Val.([]byte))
			// fmt.Println(buf.String())

			// memory, pieces, err := op.ExecuteStackProgram(op.DwarfRegisters{}, field.Val.([]byte), 8, nil)

			// fmt.Printf("%v, %v, %v\n", memory, pieces, err)
		}

	}

	ranges, err := dwarfRawData.Ranges(entry)
	if err != nil {
		panic(err)
	}
	function.lowPC = ranges[0][0]
	function.highPC = ranges[0][1]
	function.parameters = make([]*dwarfParameter, 0)

	return &function
}

func parseModule(entry *dwarf.Entry, dwarfRawData *dwarf.Data) *dwarfModule {
	module := dwarfModule{
		files:     make(map[int]string),
		functions: make([]*dwarfFunc, 0),
		variables: make([]*dwarfVariable, 0),
	}

	for _, field := range entry.Field {
		switch field.Attr {
		case dwarf.AttrName:
			module.name = field.Val.(string)
		case dwarf.AttrLanguage:
			// language can be inferred from the cu attributes. 22-golang, 12-clang
			// if field.Val.(int64) == 22 {
			// 	data.lang = golang
			// }
		case dwarf.AttrProducer:
			// can infer arch
		}
	}

	ranges, err := dwarfRawData.Ranges(entry)

	if err != nil {
		panic(err)
	}
	// might be more than 1 range entry in theory
	module.startAddress = ranges[0][0]
	module.endAddress = ranges[0][1]

	lineReader, err := dwarfRawData.LineReader(entry)
	if err != nil {
		panic(err)
	}

	moduleFileIndexMap := make(map[string]int)

	files := lineReader.Files()
	for fileIndex, file := range files {
		if file != nil {
			module.files[fileIndex] = file.Name
			moduleFileIndexMap[file.Name] = fileIndex
		}
	}

	dEntries := make([]dwarfEntry, 0)
	for {
		var le dwarf.LineEntry

		err := lineReader.Next(&le)

		if err == io.EOF {
			break
		}

		entry := dwarfEntry{
			address:       le.Address,
			file:          moduleFileIndexMap[le.File.Name],
			line:          le.Line,
			col:           le.Column,
			prologueEnd:   le.PrologueEnd,
			epilogueBegin: le.EpilogueBegin,
			isStmt:        le.IsStmt,
		}

		// adjust for inserted line
		entry.line--

		dEntries = append(dEntries, entry)

	}

	module.entries = dEntries

	return &module
}

func resolveMPIDebugInfo(data *dwarfData) dwarfMPIData {

	mpiWrapFunctions := make([]*dwarfFunc, 0)

	module, sigFunc := data.lookupFunc(MPI_FUNCS.SIGNATURE)

	for _, function := range module.functions {
		if function.file == sigFunc.file && function != sigFunc {
			function.name = function.name[1:]
			mpiWrapFunctions = append(mpiWrapFunctions, function)
		}
	}

	return dwarfMPIData{
		mpiWrapFunctions,
		module.files[sigFunc.file],
	}
}
