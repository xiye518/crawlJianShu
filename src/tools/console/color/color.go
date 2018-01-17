package color

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"log"
	"io"
	"tools/console"
)

// NoColor defines if the output is colorized or not. It's dynamically set to
// false or true based on the stdout's file descriptor referring to a terminal
// or not. This is a global option and affects all colors. For more control
// over each color block use the methods DisableColor() individually.
var NoColor = !console.IsTerminal(os.Stdout.Fd())

// Color defines a custom color object which is defined by SGR parameters.
type Color struct {
	params  []Attribute
	noColor *bool
}

// Attribute defines a single SGR Code
type Attribute int

const escape = "\x1b"

// Base attributes
const (
	Reset Attribute = iota
	Bold
	Faint
	Italic
	Underline
	BlinkSlow
	BlinkRapid
	ReverseVideo
	Concealed
	CrossedOut
)

// Foreground text colors
const (
	FgBlack Attribute = iota + 30
	FgRed
	FgGreen
	FgYellow
	FgBlue
	FgMagenta
	FgCyan
	FgWhite
)

// Foreground Hi-Intensity text colors
const (
	FgHiBlack Attribute = iota + 90
	FgHiRed
	FgHiGreen
	FgHiYellow
	FgHiBlue
	FgHiMagenta
	FgHiCyan
	FgHiWhite
)

// Background text colors
const (
	BgBlack Attribute = iota + 40
	BgRed
	BgGreen
	BgYellow
	BgBlue
	BgMagenta
	BgCyan
	BgWhite
)

// Background Hi-Intensity text colors
const (
	BgHiBlack Attribute = iota + 100
	BgHiRed
	BgHiGreen
	BgHiYellow
	BgHiBlue
	BgHiMagenta
	BgHiCyan
	BgHiWhite
)

func (a Attribute)Conv(x interface{}) *Colorize {
		return &Colorize{x,a}
}
func (a Attribute)ColorString(x interface{}) string {
	c := getCachedColor(a)
	return c.format() + fmt.Sprint(x) + c.unformat()
}
func (a Attribute)ColorFString(f string,x interface{}) string {
	c := getCachedColor(a)
	return c.format() + fmt.Sprintf(f,x) + c.unformat()
}

// New returns a newly created color object.
func New(value ...Attribute) *Color {
	c := &Color{params: make([]Attribute, 0)}
	c.Add(value...)
	return c
}

// Set sets the given parameters immediately. It will change the color of
// output with the given SGR parameters until color.Unset() is called.
func Set(p ...Attribute) *Color {
	c := New(p...)
	c.Set()
	return c
}

// Unset resets all escape attributes and clears the output. Usually should
// be called after Set().
func Unset() {
	if NoColor {
		return
	}

	fmt.Fprintf(Output, "%s[%dm", escape, Reset)
}

// Set sets the SGR sequence.
func (c *Color) Set() *Color {
	if c.isNoColorSet() {
		return c
	}

	fmt.Fprintf(Output, c.format())
	return c
}

func (c *Color) unset() {
	if c.isNoColorSet() {
		return
	}

	Unset()
}

// Add is used to chain SGR parameters. Use as many as parameters to combine
// and create custom color objects. Example: Add(color.FgRed, color.Underline).
func (c *Color) Add(value ...Attribute) *Color {
	c.params = append(c.params, value...)
	return c
}

func (c *Color) prepend(value Attribute) {
	c.params = append(c.params, 0)
	copy(c.params[1:], c.params[0:])
	c.params[0] = value
}

// Output defines the standard output of the print functions. By default
// os.Stdout is used.
var Output = console.NewColorableStdout()

// Print formats using the default formats for its operands and writes to
// standard output. Spaces are added between operands when neither is a
// string. It returns the number of bytes written and any write error
// encountered. This is the standard fmt.Print() method wrapped with the given
// color.
func (c *Color) Print(a ...interface{}) (n int, err error) {
	c.Set()
	defer c.unset()

	return fmt.Fprint(Output, a...)
}

// Printf formats according to a format specifier and writes to standard output.
// It returns the number of bytes written and any write error encountered.
// This is the standard fmt.Printf() method wrapped with the given color.
func (c *Color) Printf(format string, a ...interface{}) (n int, err error) {
	c.Set()
	defer c.unset()

	return fmt.Fprintf(Output, format, a...)
}

// Println formats using the default formats for its operands and writes to
// standard output. Spaces are always added between operands and a newline is
// appended. It returns the number of bytes written and any write error
// encountered. This is the standard fmt.Print() method wrapped with the given
// color.
func (c *Color) Println(a ...interface{}) (n int, err error) {
	c.Set()
	defer c.unset()

	return fmt.Fprintln(Output, a...)
}

// PrintFunc returns a new function that prints the passed arguments as
// colorized with color.Print().
func (c *Color) PrintFunc() func(a ...interface{}) {
	return func(a ...interface{}) { c.Print(a...) }
}

// PrintfFunc returns a new function that prints the passed arguments as
// colorized with color.Printf().
func (c *Color) PrintfFunc() func(format string, a ...interface{}) {
	return func(format string, a ...interface{}) { c.Printf(format, a...) }
}

// PrintlnFunc returns a new function that prints the passed arguments as
// colorized with color.Println().
func (c *Color) PrintlnFunc() func(a ...interface{}) {
	return func(a ...interface{}) { c.Println(a...) }
}

// SprintFunc returns a new function that returns colorized strings for the
// given arguments with fmt.Sprint(). Useful to put into or mix into other
// string. Windows users should use this in conjuction with color.Output, example:
//
//	put := New(FgYellow).SprintFunc()
//	fmt.Fprintf(color.Output, "This is a %s", put("warning"))
func (c *Color) SprintFunc() func(a ...interface{}) string {
	return func(a ...interface{}) string {
		return c.wrap(fmt.Sprint(a...))
	}
}

// SprintfFunc returns a new function that returns colorized strings for the
// given arguments with fmt.Sprintf(). Useful to put into or mix into other
// string. Windows users should use this in conjuction with color.Output.
func (c *Color) SprintfFunc() func(format string, a ...interface{}) string {
	return func(format string, a ...interface{}) string {
		return c.wrap(fmt.Sprintf(format, a...))
	}
}

// SprintlnFunc returns a new function that returns colorized strings for the
// given arguments with fmt.Sprintln(). Useful to put into or mix into other
// string. Windows users should use this in conjuction with color.Output.
func (c *Color) SprintlnFunc() func(a ...interface{}) string {
	return func(a ...interface{}) string {
		return c.wrap(fmt.Sprintln(a...))
	}
}

// sequence returns a formated SGR sequence to be plugged into a "\x1b[...m"
// an example output might be: "1;36" -> bold cyan
func (c *Color) sequence() string {
	format := make([]string, len(c.params))
	for i, v := range c.params {
		format[i] = strconv.Itoa(int(v))
	}

	return strings.Join(format, ";")
}

// wrap wraps the s string with the colors attributes. The string is ready to
// be printed.
func (c *Color) wrap(s string) string {
	if c.isNoColorSet() {
		return s
	}

	return c.format() + s + c.unformat()
}

func (c *Color) format() string {
	return fmt.Sprintf("%s[%sm", escape, c.sequence())
}

func (c *Color) unformat() string {
	return fmt.Sprintf("%s[%dm", escape, Reset)
}

// DisableColor disables the color output. Useful to not change any existing
// code and still being able to output. Can be used for flags like
// "--no-color". To enable back use EnableColor() method.
func (c *Color) DisableColor() {
	c.noColor = boolPtr(true)
}

// EnableColor enables the color output. Use it in conjuction with
// DisableColor(). Otherwise this method has no side effects.
func (c *Color) EnableColor() {
	c.noColor = boolPtr(false)
}

func (c *Color) isNoColorSet() bool {
	// check first if we have user setted action
	if c.noColor != nil {
		return *c.noColor
	}

	// if not return the global option, which is disabled by default
	return NoColor
}

// Equals returns a boolean value indicating whether two colors are equal.
func (c *Color) Equals(c2 *Color) bool {
	if len(c.params) != len(c2.params) {
		return false
	}

	for _, attr := range c.params {
		if !c2.attrExists(attr) {
			return false
		}
	}

	return true
}

func (c *Color) attrExists(a Attribute) bool {
	for _, attr := range c.params {
		if attr == a {
			return true
		}
	}

	return false
}

func boolPtr(v bool) *bool {
	return &v
}

// colorsCache is used to reduce the count of created Color objects and
// allows to reuse already created objects with required Attribute.
var colorsCache = make(map[Attribute]*Color)

var colorsCacheMu = new(sync.Mutex) // protects colorsCache

func getCachedColor(p Attribute) *Color {
	colorsCacheMu.Lock()
	defer colorsCacheMu.Unlock()

	c, ok := colorsCache[p]
	if !ok {
		c = New(p)
		colorsCache[p] = c
	}

	return c
}

func colorPrint(format string, p Attribute, a ...interface{}) {
	c := getCachedColor(p)

	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}

	if len(a) == 0 {
		c.Print(format)
	} else {
		c.Printf(format, a...)
	}
}

func colorString(format string, p Attribute, a ...interface{}) string {
	c := getCachedColor(p)

	if len(a) == 0 {
		return c.SprintFunc()(format)
	}

	return c.SprintfFunc()(format, a...)
}


// BlackString is an convenient helper function to return a string with black
// foreground.
func Black(a interface{}) *Colorize {	return &Colorize{a,FgBlack}}

// RedString is an convenient helper function to return a string with red
// foreground.
func Red(a interface{}) *Colorize { return &Colorize{a,FgRed}}

// GreenString is an convenient helper function to return a string with green
// foreground.
func Green(a interface{}) *Colorize { return &Colorize{a,FgGreen}}

// YellowString is an convenient helper function to return a string with yellow
// foreground.
func Yellow(a interface{}) *Colorize { return &Colorize{a,FgYellow}}

// BlueString is an convenient helper function to return a string with blue
// foreground.
func Blue(a interface{}) *Colorize { return &Colorize{a,FgBlue}}

// MagentaString is an convenient helper function to return a string with magenta
// foreground.
func Magenta(a interface{}) *Colorize { return &Colorize{a,FgMagenta}}

// CyanString is an convenient helper function to return a string with cyan
// foreground.
func Cyan(a interface{}) *Colorize { return &Colorize{a,FgCyan}}

// WhiteString is an convenient helper function to return a string with white
// foreground.
func White(a interface{}) *Colorize { return &Colorize{a,FgWhite}}

// BlackString is an convenient helper function to return a string with black
// foreground.
func HiBlack(a interface{}) *Colorize {	return &Colorize{a,FgHiBlack}}

// RedString is an convenient helper function to return a string with red
// foreground.
func HiRed(a interface{}) *Colorize { return &Colorize{a,FgHiRed}}

// GreenString is an convenient helper function to return a string with green
// foreground.
func HiGreen(a interface{}) *Colorize { return &Colorize{a,FgHiGreen}}

// YellowString is an convenient helper function to return a string with yellow
// foreground.
func HiYellow(a interface{}) *Colorize { return &Colorize{a,FgHiYellow}}

// BlueString is an convenient helper function to return a string with blue
// foreground.
func HiBlue(a interface{}) *Colorize { return &Colorize{a,FgHiBlue}}

// MagentaString is an convenient helper function to return a string with magenta
// foreground.
func HiMagenta(a interface{}) *Colorize { return &Colorize{a,FgHiMagenta}}

// CyanString is an convenient helper function to return a string with cyan
// foreground.
func HiCyan(a interface{}) *Colorize { return &Colorize{a,FgHiCyan}}

// WhiteString is an convenient helper function to return a string with white
// foreground.
func HiWhite(a interface{}) *Colorize { return &Colorize{a,FgHiWhite}}


type Colorize struct {
	Data interface{}
	Attribute Attribute
}
func (cz *Colorize )String()string{
	return fmt.Sprint(cz.Data)
}
//转换成有颜色的打印
func convColorParam(a []interface{})[]interface{}{
	rst:= []interface{}{}
	for _,x:= range a{
		if cz,ok:=x.(*Colorize);ok{
			c := getCachedColor(cz.Attribute)
			if NoColor{
				rst = append(rst,cz.Data)
			}else{
				rst = append(rst,c.format() + fmt.Sprint(cz.Data) + c.unformat())
			}
		}else{
			rst = append(rst,x)
		}
	}
	return rst
}

//转换成无颜色的打印
func convParam(a []interface{})[]interface{}{
	rst:= []interface{}{}
	for _,x:= range a{
		if cz,ok:=x.(*Colorize);ok{
			rst = append(rst,fmt.Sprint(cz.Data))
		}else{
			rst = append(rst,x)
		}
	}
	return rst
}

var printLock = &sync.Mutex{}

func Print(a ...interface{}){
	printLock.Lock()
	defer printLock.Unlock()
	if NoColor {
		fmt.Fprint(os.Stdout, convParam(a)...)
	}else{
		fmt.Fprint(Output, convColorParam(a)...)
	}
}
func Println(a ...interface{}) (n int, err error) {
	printLock.Lock()
	defer printLock.Unlock()
	if NoColor {
		return fmt.Fprintln(os.Stdout, convParam(a)...)
	}else{
		return fmt.Fprintln(Output, convColorParam(a)...)
	}
}

func Printf(format string, a ...interface{}) (n int, err error) {
	printLock.Lock()
	defer printLock.Unlock()
	if NoColor {
		return fmt.Fprintf(os.Stdout, format, convParam(a)...)
	}else{
		return fmt.Fprintf(Output, format, convColorParam(a)...)
	}
}


var std = log.New(os.Stderr, "", log.LstdFlags)
var logIsSetOutput bool = false
// SetOutput sets the output destination for the standard logger.
func SetLogOutput(w io.Writer) {
	logIsSetOutput = true
	std.SetOutput(w)
}

// SetFlags sets the output flags for the standard logger.
func SetLogFlags(flag int) {
	std.SetFlags(flag)
}

// SetPrefix sets the output prefix for the standard logger.
func SetLogPrefix(prefix string) {
	std.SetPrefix(prefix)
}


func LogAndPrintln(a ...interface{}) (n int, err error){
	if logIsSetOutput{
		std.Output(2, fmt.Sprintln(convParam(a)...))
	}
	return Println(a...)
}

func LogAndPrintf(format string, a ...interface{}) (n int, err error) {
	if logIsSetOutput {
		std.Output(2, fmt.Sprintf(format, convParam(a)...))
	}
	return Printf(format,a...)
}

