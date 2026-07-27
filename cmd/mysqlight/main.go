package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	mysqlight "github.com/robertkoller/MySQLight"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mysqlight <database.db>")
		os.Exit(1)
	}

	path := os.Args[1]
	database, err := mysqlight.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	fmt.Printf("MySQLight  %s\nType SQL statements ending with ; or .exit to quit.\n\n", path)

	scanner := bufio.NewScanner(os.Stdin)
	var inputBuilder strings.Builder

	for {
		if inputBuilder.Len() == 0 {
			fmt.Print("mysqlight> ")
		} else {
			fmt.Print("        -> ")
		}

		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == ".exit" || trimmedLine == ".quit" || trimmedLine == "\\q" {
			break
		}

		if trimmedLine == "" {
			continue
		}

		inputBuilder.WriteString(line)
		inputBuilder.WriteByte(' ')

		// Wait for a semicolon before executing.
		if !strings.Contains(trimmedLine, ";") {
			continue
		}

		sql := strings.TrimSpace(inputBuilder.String())
		sql = strings.TrimRight(sql, "; ")
		inputBuilder.Reset()

		if sql == "" {
			continue
		}

		result, err := database.Exec(sql)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}

		printResult(result)
	}

	fmt.Println("\nbye.")
}

func printResult(result mysqlight.Result) {
	if len(result.Columns) == 0 {
		fmt.Println("OK")
		return
	}

	if len(result.Rows) == 0 {
		// Print headers then an empty result notice.
		printHeader(result.Columns)
		fmt.Println("(0 rows)")
		return
	}

	// Convert all values to strings and compute column widths.
	widths := make([]int, len(result.Columns))
	for index, column := range result.Columns {
		widths[index] = len(column)
	}

	formattedRows := make([][]string, len(result.Rows))
	for rowIndex, row := range result.Rows {
		formattedRows[rowIndex] = make([]string, len(row))
		for colIndex, value := range row {
			var columnType mysqlight.DataType
			if colIndex < len(result.ColumnTypes) {
				columnType = result.ColumnTypes[colIndex]
			}
			formatted := mysqlight.FormatValue(value, columnType)
			formattedRows[rowIndex][colIndex] = formatted
			if len(formatted) > widths[colIndex] {
				widths[colIndex] = len(formatted)
			}
		}
	}

	printHeader(result.Columns)
	printSeparator(widths)

	for _, row := range formattedRows {
		for colIndex, cell := range row {
			if colIndex > 0 {
				fmt.Print("  ")
			}
			fmt.Printf("%-*s", widths[colIndex], cell)
		}
		fmt.Println()
	}

	rowWord := "rows"
	if len(result.Rows) == 1 {
		rowWord = "row"
	}
	fmt.Printf("(%d %s)\n", len(result.Rows), rowWord)
}

func printHeader(columns []string) {
	for index, column := range columns {
		if index > 0 {
			fmt.Print("  ")
		}
		fmt.Print(column)
	}
	fmt.Println()
}

func printSeparator(widths []int) {
	for index, width := range widths {
		if index > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.Repeat("-", width))
	}
	fmt.Println()
}
