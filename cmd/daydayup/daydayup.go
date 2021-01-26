package main

import (
	"fmt"

	"github.com/xiye518/crawjianshu/internal/tools/console/color"
)

func init() {
	color.NoColor = false
}

func main() {
	var a, b = 1.00, 1.01
	var count int
	for a < 2.00 {
		count++
		a *= b
		fmt.Println(b, a)
	}

	color.Println(color.HiCyan("\nGet A little bit of progress everyday,how many days will it take to double:"), color.HiGreen(count)) //70
	//fmt.Println("\nGet A little bit of progress everyday,how many days will it take to double:", count) //70
}
