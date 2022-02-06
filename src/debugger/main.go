package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/ottmartens/cc-rev-db/logger"
)

type processContext struct {
	sourceFile string          // source code file
	dwarfData  *dwarfData      //
	process    *exec.Cmd       // the running binary
	pid        int             // the process id of the running binary
	bpointData *breakpointData // holds the instuctions for currently replaced by breakpoints
	// lang
}

// type lang string

// const (
// 	golang lang = "go"
// 	c      lang = "c"
// )

type breakpointData struct {
	userBpoints map[int]*bpointData       // user-inserted breakpoints. keys - line numbers
	mpiBpoints  map[uint64]*mpiBpointData // auto-inserted breakpoints keys -> addresses
}

type mpiBpointData struct {
	data     []byte
	function *dwarfFunc
}

type bpointData struct {
	address uint64 // address of the instruction
	data    []byte // actual contents of the instruction
}

func main() {
	targetFile := getValuesFromArgs()

	ctx := &processContext{}

	ctx.dwarfData = getDwarfData(targetFile)

	ctx.sourceFile = getSourceFileInfo(ctx.dwarfData)
	ctx.bpointData = breakpointData{}.New()

	ctx.process = startBinary(targetFile)
	ctx.pid = ctx.process.Process.Pid

	insertMPIBreakpoints(ctx)

	printInstructions()

	for {
		cmd := askForInput()

		res := cmd.handle(ctx)

		if res.exited { // binary exited
			break
		}

		if cmd.isProgressCommand() {
			restoreCaughtBreakpoint(ctx)
			logRegistersState(ctx)
		}
	}
}

func startBinary(target string) *exec.Cmd {

	cmd := exec.Command(target)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Ptrace: true,
	}

	cmd.Start()
	err := cmd.Wait()

	if err != nil {
		// arrived at auto-inserted initial breakpoint trap
		logger.Info("binary started, waiting for continuation")
	}

	return cmd
}

func getSourceFileInfo(d *dwarfData) (sourceFile string) {
	// languageEntryFuncs := map[lang]string{
	// 	golang: "main.main",
	// 	c:      "main",
	// }

	entryFunc := "main"

	module, function := d.lookupFunc(entryFunc)
	// if module != nil {
	// 	language = golang
	// } else {
	// 	module, function = d.lookupFunc(languageEntryFuncs[c])
	// 	language = c
	// }

	sourceFile = module.files[function.file]

	return sourceFile
}

func logRegistersState(ctx *processContext) {
	regs := getRegs(ctx, false)

	line, fileName, _, _ := ctx.dwarfData.PCToLine(regs.Rip)

	logger.Info("instruction pointer: %x (line %d in %s)\n", regs.Rip, line, fileName)

	data := make([]byte, 4)
	syscall.PtracePeekData(ctx.pid, uintptr(regs.Rip), data)
	logger.Info("ip pointing to: %v\n", data)
}

func getRegs(ctx *processContext, rewindIP bool) *syscall.PtraceRegs {
	var regs syscall.PtraceRegs

	err := syscall.PtraceGetRegs(ctx.pid, &regs)

	if err != nil {
		fmt.Printf("getregs error: %v\n", err)
	}

	// if currently stopped by a breakpoint, rewind the instruction pointer by 1
	// to find the correct instruction (rewind the interrupt instruction)
	if rewindIP {
		regs.Rip -= 1
	}

	return &regs
}

// parse and validate command line arguments
func getValuesFromArgs() string {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug <target binary>")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "mpi":
		logger.Info("mpi specified, loading example mpi binary")
		return "examples/hello_mpi_c/hello"
	case "c":
		logger.Info("c specified, loading example c binary")
		return "examples/hello_c/hello"
	case "go":
		logger.Info("go specified, loading example c binary")
		return "examples/hello_go/hello"
	}

	targetFilePath, err := filepath.Abs(os.Args[1])

	if err != nil {
		panic(err)
	}

	if _, err := os.Stat(targetFilePath); errors.Is(err, os.ErrNotExist) {
		panic(err) // file does not exist
	}

	return targetFilePath
}

func (b breakpointData) New() *breakpointData {

	return &breakpointData{
		userBpoints: make(map[int]*bpointData),
		mpiBpoints:  make(map[uint64]*mpiBpointData),
	}
}
