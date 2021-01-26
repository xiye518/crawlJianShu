package main

import (
	"log"
	"os"

	"github.com/xiye518/crawjianshu/internal/tools/console/color"
)

func init() {
	color.SetLogOutput(os.Stdout) //默认Stderr 在idea的console中是非线程安全的
	color.NoColor = false
	color.SetLogFlags(log.Lshortfile | log.Ldate | log.Lmicroseconds)
}

func main() {
	//EnableColor()
	color.LogAndPrintln("Prints", color.HiCyan("cyan"), "text with an underline.")

}
