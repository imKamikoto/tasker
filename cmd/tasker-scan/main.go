// Command tasker-scan — отладочный CLI поверх ядра: строит индекс по папке и
// отвечает на запросы, не поднимая ни окна, ни вебвью.
//
// Он существует не ради удобства, а как доказательство того, что граница
// проведена правильно: если поиск работает отсюда, значит логика действительно
// в Go. В день, когда tasker-scan перестанет собираться без фронтенда, граница
// поехала.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: tasker-scan <vault> [query]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	// Фаза 1: индексация и поиск. См. docs/ROADMAP.md.
	fmt.Fprintln(os.Stderr, "tasker-scan: не реализовано, фаза 1")
	os.Exit(1)
}
